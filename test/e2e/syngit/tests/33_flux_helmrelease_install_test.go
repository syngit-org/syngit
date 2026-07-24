/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fluxprovider "github.com/syngit-org/syngit-provider-flux/pkg"
	helmprovider "github.com/syngit-org/syngit-provider-helm/pkg"
	"github.com/syngit-org/syngit/internal/mutator"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	testutils "github.com/syngit-org/syngit/test/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("33 FluxHelmRelease install synthesizes a HelmRelease", func() {

	const (
		releaseName = "demo"
		secretName  = "sh.helm.release.v1." + releaseName + ".v1"
	)

	// helmReleaseSyncer builds a RemoteSyncer scoped to Helm release secrets with
	// ResourceFinder enabled, mirroring the helm-values install specs.
	helmReleaseSyncer := func(fx *utils.Fixture, name string) *syngit.RemoteSyncer {
		return &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: "main",
					fluxprovider.HelmReleaseAnnotation:      "enabled",
					helmprovider.HelmValuesAnnotation:       "enabled",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            fx.RepoURL(),
				ResourceFinder:              true,
				// Server-managed fields carried over from the live (cluster) donor
				// HelmRelease must be stripped before the manifest is committed.
				ExcludedFields: []string{
					".metadata.uid",
					".metadata.resourceVersion",
					".metadata.creationTimestamp",
					".metadata.generation",
					".metadata.managedFields",
					".status",
				},
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create, admissionv1.Update, admissionv1.Delete,
						},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"secrets"},
						},
					}},
					ObjectSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"owner": "helm"},
					},
				},
			},
		}
	}

	// installDummyChart installs the dummy chart as Developer with the given values
	// override file content.
	installDummyChart := func(fx *utils.Fixture, valuesOverride string) {
		overridePath := filepath.Join(GinkgoT().TempDir(), "values.yaml")
		Expect(os.WriteFile(overridePath, []byte(valuesOverride), 0o600)).To(Succeed())

		chart := testutils.LocalChart{
			ChartPath: filepath.Join(utils.ProjectRoot(),
				"test", "e2e", "samples", "charts"),
			BaseChart: testutils.BaseChart{
				ValuesPath:       overridePath,
				ChartName:        "dummy",
				ChartVersion:     "dummy",
				ReleaseName:      releaseName,
				ReleaseNamespace: fx.Namespace,
			},
		}
		actionCfg, settings, err := testutils.NewEnvtestHelmActionConfig(
			fx.Users.CfgAs(utils.Developer), fx.Namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(testutils.InstallChart(chart, actionCfg, settings)).To(Succeed())
	}

	// donorHelmRelease builds the existing HelmRelease that lives in the cluster and
	// serves as the donor for ConvertToHelmReleaseWithExisting: its whole spec
	// (interval, chart, sourceRef) is preserved while only spec.values is overridden.
	// It carries no releaseName and a status, to exercise both the preserve and the
	// excluded-fields stripping behavior.
	donorHelmRelease := func(fx *utils.Fixture) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "helm.toolkit.fluxcd.io/v2",
			"kind":       "HelmRelease",
			"metadata": map[string]interface{}{
				"name":      releaseName,
				"namespace": fx.Namespace,
			},
			"spec": map[string]interface{}{
				"interval": "1h",
				"chart": map[string]interface{}{
					"spec": map[string]interface{}{
						"chart": "dummy",
						"sourceRef": map[string]interface{}{
							"kind":      "HelmRepository",
							"name":      "dummy-repo",
							"namespace": fx.Namespace,
						},
					},
				},
			},
			"status": map[string]interface{}{
				"observedGeneration": int64(1),
			},
		}}
	}

	It("synthesizes a HelmRelease at the default path, preserving the cluster donor's spec", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		defaultHrPath := fmt.Sprintf("%s/helm.toolkit.fluxcd.io/v2/helmreleases/%s.yaml",
			fx.Namespace, secretName)

		By("creating the existing (donor) HelmRelease in the cluster")
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx, donorHelmRelease(fx))).To(Succeed())

		By("creating the RemoteUser & RemoteUserBinding for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer scoped to Helm release secrets")
		rs := helmReleaseSyncer(fx, "remotesyncer-test33-default-path")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("installing the dummy chart as Developer against envtest")
		installDummyChart(fx, "data:\n  greeting: hello\n")

		By("checking the synthesized HelmRelease was pushed at the default structured path")
		Eventually(func(g Gomega) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", defaultHrPath)
			g.Expect(err).NotTo(HaveOccurred(),
				"expected the synthesized HelmRelease at %q", defaultHrPath)
			body := string(content)
			g.Expect(body).To(ContainSubstring("kind: HelmRelease"))
			// The donor's spec is preserved; only spec.values is overridden.
			g.Expect(body).To(ContainSubstring("interval: 1h"),
				"the donor's spec.interval must be preserved")
			g.Expect(body).To(ContainSubstring("name: dummy-repo"),
				"the donor's chart sourceRef must be preserved")
			g.Expect(body).To(ContainSubstring("kind: HelmRepository"))
			g.Expect(body).To(ContainSubstring("greeting"),
				"the synthesized HelmRelease must carry the install-supplied values")
			// ConvertToHelmReleaseWithExisting copies the donor verbatim; it had no
			// releaseName, so none must be invented.
			g.Expect(body).NotTo(ContainSubstring("releaseName:"),
				"the donor had no releaseName, so none should be synthesized")
			// The RemoteSyncer's excluded server-managed fields are stripped.
			for _, field := range []string{"uid:", "resourceVersion:", "status:", "creationTimestamp:"} {
				g.Expect(body).NotTo(ContainSubstring(field),
					"excluded server-managed field %q must be stripped", field)
			}
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())
	})

	It("does not push a HelmRelease when none exists in the cluster", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		defaultHrPath := fmt.Sprintf("%s/helm.toolkit.fluxcd.io/v2/helmreleases/%s.yaml",
			fx.Namespace, secretName)
		valuesPath := fmt.Sprintf("%s/%s/%s.yaml",
			fx.Namespace, mutator.DefaultChartValuesSubPath, secretName)

		By("creating the RemoteUser & RemoteUserBinding for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer scoped to Helm release secrets (no pre-existing HelmRelease in the cluster)")
		rs := helmReleaseSyncer(fx, "remotesyncer-test33-absent")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("installing the dummy chart as Developer against envtest")
		installDummyChart(fx, "data:\n  greeting: hello\n")

		By("checking the install was intercepted (the values file is committed)")
		Eventually(func(g Gomega) {
			exists, err := fx.Git.FileExists(fx.Repo, "main", valuesPath)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(exists).To(BeTrue(),
				"expected the chart-values file at %q, proving the secret was intercepted", valuesPath)
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())

		By("checking no HelmRelease was ever pushed")
		Consistently(func() (bool, error) {
			return fx.Git.FileExists(fx.Repo, "main", defaultHrPath)
		}).WithTimeout(3*utils.DefaultInterval).
			Should(BeFalse(),
				"the provider found no existing HelmRelease in the cluster, so none should be pushed")
	})

	// existingHelmReleaseFile builds the HelmRelease manifest a user already
	// manages in their GitOps repo, keyed by the release's own identity.
	existingHelmReleaseFile := func(fx *utils.Fixture) []byte {
		return []byte(fmt.Sprintf(`apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: %s
  namespace: %s
spec:
  interval: 1h
  values:
    data:
      greeting: previous
`, releaseName, fx.Namespace))
	}

	It("overrides an existing HelmRelease file in the repo instead of writing the default path", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const customPath = "clusters/prod/demo.yaml"
		defaultHrPath := fmt.Sprintf("%s/helm.toolkit.fluxcd.io/v2/helmreleases/%s.yaml",
			fx.Namespace, secretName)

		By("pre-committing an existing HelmRelease file at a custom repo path")
		Expect(fx.Git.CommitFile(fx.Repo, "main", customPath,
			existingHelmReleaseFile(fx), "seed existing HelmRelease")).To(Succeed())

		By("creating the existing (donor) HelmRelease in the cluster")
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx, donorHelmRelease(fx))).To(Succeed())

		By("creating the RemoteUser & RemoteUserBinding for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer scoped to Helm release secrets")
		rs := helmReleaseSyncer(fx, "remotesyncer-test33-in-repo")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("installing the dummy chart as Developer against envtest")
		installDummyChart(fx, "data:\n  greeting: hello\n")

		By("checking the existing HelmRelease file was updated in place")
		Eventually(func(g Gomega) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", customPath)
			g.Expect(err).NotTo(HaveOccurred(),
				"expected the pre-existing HelmRelease at %q", customPath)
			body := string(content)
			g.Expect(body).To(ContainSubstring("greeting: hello"),
				"the existing HelmRelease must carry the install-supplied values")
			g.Expect(body).NotTo(ContainSubstring("greeting: previous"),
				"the old values must be replaced in place")
			// The donor's spec is still preserved through the synthesis.
			g.Expect(body).To(ContainSubstring("name: dummy-repo"),
				"the donor's chart sourceRef must be preserved")
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())

		By("checking no HelmRelease was written at the generated default path")
		Consistently(func() (bool, error) {
			return fx.Git.FileExists(fx.Repo, "main", defaultHrPath)
		}).WithTimeout(3*utils.DefaultInterval).
			Should(BeFalse(),
				"resource-finder must override the existing file, not duplicate it at %q", defaultHrPath)
	})

	It("removes the HelmRelease from the existing repo file on uninstall", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const customPath = "clusters/prod/demo.yaml"

		By("pre-committing an existing HelmRelease file at a custom repo path")
		Expect(fx.Git.CommitFile(fx.Repo, "main", customPath,
			existingHelmReleaseFile(fx), "seed existing HelmRelease")).To(Succeed())

		By("creating the existing (donor) HelmRelease in the cluster")
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx, donorHelmRelease(fx))).To(Succeed())

		By("creating the RemoteUser & RemoteUserBinding for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer scoped to Helm release secrets")
		rs := helmReleaseSyncer(fx, "remotesyncer-test33-uninstall")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("installing the dummy chart as Developer against envtest")
		overridePath := filepath.Join(GinkgoT().TempDir(), "values.yaml")
		Expect(os.WriteFile(overridePath, []byte("data:\n  greeting: hello\n"), 0o600)).To(Succeed())
		chart := testutils.LocalChart{
			ChartPath: filepath.Join(utils.ProjectRoot(),
				"test", "e2e", "samples", "charts"),
			BaseChart: testutils.BaseChart{
				ValuesPath:       overridePath,
				ChartName:        "dummy",
				ChartVersion:     "dummy",
				ReleaseName:      releaseName,
				ReleaseNamespace: fx.Namespace,
			},
		}
		actionCfg, settings, err := testutils.NewEnvtestHelmActionConfig(
			fx.Users.CfgAs(utils.Developer), fx.Namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(testutils.InstallChart(chart, actionCfg, settings)).To(Succeed())

		By("waiting for the existing HelmRelease file to be updated in place")
		Eventually(func(g Gomega) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", customPath)
			g.Expect(err).NotTo(HaveOccurred(),
				"expected file %q on main before uninstall", customPath)
			g.Expect(string(content)).To(ContainSubstring("greeting: hello"),
				"file at %q must reflect the install override before uninstall", customPath)
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())

		By("uninstalling the dummy chart as Developer")
		Expect(testutils.UninstallChart(chart, actionCfg, settings)).To(Succeed())

		By("checking the HelmRelease file has been removed from the repo")
		Eventually(func(g Gomega) {
			exists, err := fx.Git.FileExists(fx.Repo, "main", customPath)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(exists).To(BeFalse(),
				"expected the HelmRelease file at %q to be removed after uninstall", customPath)
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())
	})
})
