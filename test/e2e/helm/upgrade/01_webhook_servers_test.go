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
	syngitutils "github.com/syngit-org/syngit/pkg/utils"
	"github.com/syngit-org/syngit/test/utils"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	samplePath = "test/e2e/build/samples"
)

var syngitChart utils.LocalChart

var remoteSyncerGVR schema.GroupVersionResource

var configMapGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "configmaps",
}

var _ = BeforeEach(func() {
	By("getting the previous chart version for initial install")
	previousVersion, err := utils.GetPreviousChartVersion("charts")
	ExpectWithOffset(2, err).NotTo(HaveOccurred())

	syngitChart = utils.LocalChart{
		ChartPath: "charts",
		BaseChart: utils.BaseChart{
			ValuesPath:       "test/e2e/helm/values.yaml",
			ChartName:        previousVersion, // Use previous version for initial install
			ReleaseName:      "syngit",
			ReleaseNamespace: "syngit",
			ChartVersion:     previousVersion, // Use previous version for initial install
		},
	}

	By("getting the latest API version")
	version, err := utils.GetLatestAPIVersion()
	ExpectWithOffset(2, err).NotTo(HaveOccurred())

	remoteSyncerGVR = schema.GroupVersionResource{
		Group:    "syngit.io",
		Version:  version,
		Resource: "remotesyncers",
	}
})

var _ = Describe("01 Test webhook servers", Ordered, func() {

	It("should run successfully", func() {

		By("installing the syngit chart")
		actionConfig, settings, err := utils.NewDefaultHelmActionConfig(syngitChart)
		ExpectWithOffset(2, err).NotTo(HaveOccurred())
		err = utils.InstallChart(syngitChart, actionConfig, settings)
		ExpectWithOffset(2, err).NotTo(HaveOccurred())
		Wait15()

		By("getting the latest API version")
		version, err := utils.GetLatestAPIVersion()
		ExpectWithOffset(2, err).NotTo(HaveOccurred())

		remoteSyncerGVR = schema.GroupVersionResource{
			Group:    "syngit.io",
			Version:  version,
			Resource: "remotesyncers",
		}

		By("creating the RemoteSyncer")
		config, err := utils.GetKubeConfig()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		err = utils.ApplyFromYAML(
			config,
			fmt.Sprintf("%s/syngit_v1beta3_remotesyncer.yaml", samplePath),
			testNamespace,
			remoteSyncerGVR,
		)
		ExpectWithOffset(2, err).NotTo(HaveOccurred())

		Wait5()

		By("upgrading to the latest chart version")
		latestVersion, err := utils.GetLatestChartVersion("charts")
		ExpectWithOffset(2, err).NotTo(HaveOccurred())

		// Update chart version before upgrade
		syngitChart.ChartName = latestVersion
		syngitChart.ChartVersion = latestVersion

		err = utils.UpgradeChart(syngitChart, actionConfig, settings)
		ExpectWithOffset(2, err).NotTo(HaveOccurred())

		By("creating a ConfigMap")
		Wait60()
		err = utils.ApplyFromYAML(
			config,
			fmt.Sprintf("%s/sample_configmap.yaml", samplePath),
			testNamespace,
			configMapGVR,
		)
		ExpectWithOffset(2, err).To(HaveOccurred())
		Expect(syngitutils.ErrorTypeChecker(&syngitutils.RemoteUserBindingNotFoundError{}, err.Error())).To(BeTrue())

		By("creating a RemoteSyncer")
		cmd := exec.Command("kubectl", "apply", "-n", testNamespace, "-f",
			fmt.Sprintf("%s/syngit_v1beta3_remotesyncer.yaml", samplePath))
		_, err = utils.Run(cmd)
		ExpectWithOffset(2, err).NotTo(HaveOccurred())

		By("creating a ConfigMap")
		Wait5()
		cmd = exec.Command("kubectl", "apply", "-n", testNamespace, "-f",
			fmt.Sprintf("%s/sample_configmap.yaml", samplePath))
		_, err = utils.Run(cmd)
		ExpectWithOffset(2, err).To(HaveOccurred())
		Expect(syngitutils.ErrorTypeChecker(&syngitutils.RemoteUserBindingNotFoundError{}, err.Error())).To(BeTrue())

	})

})

var _ = AfterEach(func() {

	By("uninstalling the syngit chart")
	actionConfig, settings, err := utils.NewDefaultHelmActionConfig(syngitChart)
	ExpectWithOffset(2, err).NotTo(HaveOccurred())
	err = utils.UninstallChart(syngitChart, actionConfig, settings)
	ExpectWithOffset(2, err).NotTo(HaveOccurred())

})
