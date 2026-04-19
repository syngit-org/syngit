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

package e2e_helm_install

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/test/utils"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	samplePath = "test/e2e/samples"
)

var syngitChart utils.LocalChart

var remoteSyncerGVR schema.GroupVersionResource

var configMapGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "configmaps",
}

var _ = BeforeEach(func() {
	syngitChart = utils.LocalChart{
		ChartPath: "charts",
		BaseChart: utils.BaseChart{
			ValuesPath:       "test/e2e/helm/values.yaml",
			ChartName:        "syngit",
			ReleaseName:      "syngit",
			ReleaseNamespace: "syngit",
			ChartVersion:     "syngit",
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

		By("creating a RemoteSyncer")
		config, err := utils.GetKubeConfig()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		version, err := utils.GetLatestAPIVersion()
		ExpectWithOffset(2, err).NotTo(HaveOccurred())

		Eventually(func() error {
			return utils.ApplyFromYAML(
				config,
				fmt.Sprintf("%s/syngit_%s_remotesyncer.yaml", samplePath, version),
				testNamespace,
				remoteSyncerGVR,
			)
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("creating a ConfigMap")
		Wait5()
		err = utils.ApplyFromYAML(
			config,
			fmt.Sprintf("%s/sample_configmap.yaml", samplePath),
			testNamespace,
			configMapGVR,
		)
		ExpectWithOffset(2, err).To(HaveOccurred())
		Expect(syngiterrors.Is(err, syngiterrors.ErrRemoteUserBindingNotFound)).To(BeTrue())
	})

})

var _ = AfterEach(func() {

	By("uninstalling the syngit chart")
	actionConfig, settings, err := utils.NewDefaultHelmActionConfig(syngitChart)
	ExpectWithOffset(2, err).NotTo(HaveOccurred())
	// Ignore uninstall errors: Helm v4 WaitForDelete may time out if the
	// controller recreates cluster-scoped resources during teardown.
	// AfterSuite handles full cleanup by deleting the namespace.
	_ = utils.UninstallChart(syngitChart, actionConfig, settings)
})
