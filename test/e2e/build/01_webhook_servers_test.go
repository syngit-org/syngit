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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/syngit-org/syngit/test/utils"
)

const (
	samplePath = "test/e2e/build/samples"
)

var _ = Describe("01 Test webhook servers", Ordered, func() {

	It("should run successfully", func() {

		By("creating a RemoteSyncer")
		Wait20()
		cmd := exec.Command("kubectl", "apply", "-n", testNamespace, "-f",
			fmt.Sprintf("%s/syngit_v1beta3_remotesyncer.yaml", samplePath))
		_, err := utils.Run(cmd)
		ExpectWithOffset(2, err).NotTo(HaveOccurred())

		By("creating a ConfigMap")
		Wait10()
		cmd = exec.Command("kubectl", "apply", "-n", testNamespace, "-f",
			fmt.Sprintf("%s/sample_configmap.yaml", samplePath))
		_, err = utils.Run(cmd)
		ExpectWithOffset(2, err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no RemoteUserBinding found for the user"))

	})

})

var _ = AfterEach(func() {
	By("cleaning up resources after each test")

	// Delete RemoteSyncer
	cmd := exec.Command("kubectl", "delete", "-n", testNamespace, "-f",
		fmt.Sprintf("%s/syngit_v1beta3_remotesyncer.yaml", samplePath))
	_, err := utils.Run(cmd)
	ExpectWithOffset(2, err).NotTo(HaveOccurred())

})
