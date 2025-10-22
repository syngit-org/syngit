/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e_build

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/syngit-org/syngit/test/utils"
)

const (
	namespace     = "syngit"
	testNamespace = "test"
)

// Run e2e tests using the Ginkgo runner.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, err := fmt.Fprintf(GinkgoWriter, "Starting syngit build suite\n")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	RunSpecs(t, "e2e suite")
}

const projectimage = "local/syngit-controller:dev"

var _ = BeforeSuite(func() {

	By("creating manager namespace")
	cmd := exec.Command("kubectl", "create", "ns", namespace)
	_, errNs := utils.Run(cmd)
	if errNs != nil && !strings.Contains(errNs.Error(), "already exists") {
		ExpectWithOffset(1, errNs).NotTo(HaveOccurred())
	}

	By("creating test namespace")
	cmd = exec.Command("kubectl", "create", "ns", testNamespace)
	_, errNs = utils.Run(cmd)
	if errNs != nil && !strings.Contains(errNs.Error(), "already exists") {
		ExpectWithOffset(1, errNs).NotTo(HaveOccurred())
	} else {
		cmd = exec.Command("kubectl", "delete", "ns", testNamespace)
		_, errDelNs := utils.Run(cmd)
		ExpectWithOffset(1, errDelNs).NotTo(HaveOccurred())
		cmd = exec.Command("kubectl", "create", "ns", testNamespace)
		_, errNs = utils.Run(cmd)
		ExpectWithOffset(1, errNs).NotTo(HaveOccurred())
	}

	var controllerPodName string
	var err error

	By("building the manager(Operator) image")
	cmd = exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectimage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("loading the the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectimage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("deploying the controller-manager")
	cmd = exec.Command("make", "deploy-all")
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("validating that the controller-manager pod is running as expected")
	verifyControllerUp := func() error {
		// Get pod name

		cmd = exec.Command("kubectl", "get",
			"pods", "-l", "control-plane=controller-manager",
			"-o", "go-template={{ range .items }}"+
				"{{ if not .metadata.deletionTimestamp }}"+
				"{{ .metadata.name }}"+
				"{{ \"\\n\" }}{{ end }}{{ end }}",
			"-n", namespace,
		)

		podOutput, err := utils.Run(cmd)
		ExpectWithOffset(2, err).NotTo(HaveOccurred())
		podNames := utils.GetNonEmptyLines(string(podOutput))
		if len(podNames) != 1 {
			return fmt.Errorf("expect 1 controller pods running, but got %d", len(podNames))
		}
		controllerPodName = podNames[0]
		ExpectWithOffset(2, controllerPodName).Should(ContainSubstring("controller-manager"))

		// Validate pod status
		cmd = exec.Command("kubectl", "get",
			"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
			"-n", namespace,
		)
		status, err := utils.Run(cmd)
		ExpectWithOffset(2, err).NotTo(HaveOccurred())
		if string(status) != "Running" {
			return fmt.Errorf("controller pod in %s status", status)
		}
		return nil
	}
	EventuallyWithOffset(1, verifyControllerUp, time.Minute, time.Second).Should(Succeed())

})

var _ = AfterSuite(func() {

	By("undeploying the controller-manager")
	cmd := exec.Command("make", "undeploy")
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("removing manager namespace")
	cmd = exec.Command("kubectl", "delete", "ns", namespace)
	_, _ = utils.Run(cmd)

	By("removing test namespace")
	cmd = exec.Command("kubectl", "delete", "ns", testNamespace)
	_, _ = utils.Run(cmd)
})

// Wait20 sleeps for 20 seconds
func Wait20() {
	time.Sleep(20 * time.Second)
}

// Wait10 sleeps for 10 seconds
func Wait10() {
	time.Sleep(10 * time.Second)
}
