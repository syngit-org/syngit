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

	"github.com/syngit-org/syngit/internal/mutator"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	testutils "github.com/syngit-org/syngit/test/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("33 FluxHelmRelease install synthesizes a HelmRelease", func() {

	const (
		releaseName = "demo"
		secretName  = "sh.helm.release.v1." + releaseName + ".v1"
		hrPath      = "apps/demo/helmrelease.yaml"
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
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create, admissionv1.Update,
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
	installDummyChart := func(ctx context.Context, fx *utils.Fixture, valuesOverride string) {
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

	It("overwrites the existing HelmRelease in place with the synthesized one", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		defaultHrPath := fmt.Sprintf("%s/helm.toolkit.fluxcd.io/v2/helmreleases/%s.yaml",
			fx.Namespace, secretName)

		By("pre-committing an existing HelmRelease that references a HelmRepository")
		existingHR := []byte(fmt.Sprintf(`apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: %s
  namespace: %s
spec:
  interval: 1h
  chart:
    spec:
      chart: dummy
      sourceRef:
        kind: HelmRepository
        name: dummy-repo
        namespace: %s
`, releaseName, fx.Namespace, fx.Namespace))
		Expect(fx.Git.CommitFile(fx.Repo, "main", hrPath, existingHR,
			"seed existing HelmRelease")).To(Succeed())

		By("creating the RemoteUser & RemoteUserBinding for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer scoped to Helm release secrets")
		rs := helmReleaseSyncer(fx, "remotesyncer-test33-in-place")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("installing the dummy chart as Developer against envtest")
		installDummyChart(ctx, fx, "data:\n  greeting: hello\n")

		By("checking the existing HelmRelease file was rewritten in place with the synthesized one")
		Eventually(func(g Gomega) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", hrPath)
			g.Expect(err).NotTo(HaveOccurred(),
				"expected the pre-committed HelmRelease %q to still exist on main", hrPath)
			body := string(content)
			g.Expect(body).To(ContainSubstring("kind: HelmRelease"))
			g.Expect(body).To(ContainSubstring("releaseName: "+releaseName),
				"synthesized HelmRelease must carry the release name (the donor had none)")
			g.Expect(body).To(ContainSubstring("name: dummy-repo"),
				"synthesized HelmRelease must carry the sourceRef copied from the existing one")
			g.Expect(body).To(ContainSubstring("kind: HelmRepository"))
			g.Expect(body).To(ContainSubstring("greeting"),
				"synthesized HelmRelease must carry the install-supplied values")
			g.Expect(body).NotTo(ContainSubstring("interval: 1h"),
				"the donor-only spec must have been replaced by the synthesized one")
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())

		By("checking no HelmRelease was created at the default structured path")
		exists, err := fx.Git.FileExists(fx.Repo, "main", defaultHrPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeFalse(),
			"the HelmRelease was written in place, so no fallback file at %q should exist", defaultHrPath)
	})

	It("does not push a HelmRelease when none exists in the repo", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		defaultHrPath := fmt.Sprintf("%s/helm.toolkit.fluxcd.io/v2/helmreleases/%s.yaml",
			fx.Namespace, secretName)
		valuesPath := fmt.Sprintf("%s/%s/%s.yaml",
			fx.Namespace, mutator.DefaultChartValuesSubPath, secretName)

		By("creating the RemoteUser & RemoteUserBinding for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer scoped to Helm release secrets (no pre-existing HelmRelease)")
		rs := helmReleaseSyncer(fx, "remotesyncer-test33-absent")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("installing the dummy chart as Developer against envtest")
		installDummyChart(ctx, fx, "data:\n  greeting: hello\n")

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
				"the provider found no existing HelmRelease, so none should be pushed")
	})
})
