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

var _ = Describe("32 HelmValuesMutation uninstall removes the values file", func() {

	It("deletes the committed values file at the default chart-values path (no ResourceFinder)", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const releaseName = "demo"

		secretName := "sh.helm.release.v1." + releaseName + ".v1"
		expectedPath := fmt.Sprintf("%s/%s/%s.yaml", fx.Namespace, mutator.DefaultChartValuesSubPath, secretName)

		By("creating the RemoteUser & RemoteUserBinding for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer scoped to Helm release secrets without ResourceFinder")
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test34-default",
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
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("writing the per-spec values override file")
		overridePath := filepath.Join(GinkgoT().TempDir(), "values.yaml")
		Expect(os.WriteFile(overridePath, []byte(
			"replicaCount: 3\ndata:\n  greeting: hello\n"), 0o600)).To(Succeed())

		By("installing the dummy chart as Developer against envtest")
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

		By("waiting for the values file to be committed at the default chart-values path")
		Eventually(func(g Gomega) {
			exists, err := fx.Git.FileExists(fx.Repo, "main", expectedPath)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(exists).To(BeTrue(),
				"expected committed file at %q on main before uninstall", expectedPath)
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())

		By("uninstalling the dummy chart as Developer")
		Expect(testutils.UninstallChart(chart, actionCfg, settings)).To(Succeed())

		By("checking that the values file has been removed from the repo")
		Eventually(func(g Gomega) {
			exists, err := fx.Git.FileExists(fx.Repo, "main", expectedPath)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(exists).To(BeFalse(),
				"expected values file at %q to be removed from main after uninstall", expectedPath)
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())
	})

	It("deletes the pre-committed values file at the custom path (with ResourceFinder)", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const (
			releaseName = "demo"
			customPath  = "apps/demo/values.yaml"
			markerLine  = "# syngit.resource-finder/v1: "
		)

		secretName := "sh.helm.release.v1." + releaseName + ".v1"
		preCommittedMarker := markerLine + fx.Namespace + "/" + secretName

		By("pre-committing a raw values.yaml at the custom path with the resource-finder marker")
		preCommitted := []byte(
			preCommittedMarker + "\n" +
				"replicaCount: 1\n" +
				"data:\n" +
				"  greeting: previous\n")
		Expect(fx.Git.CommitFile(fx.Repo, "main", customPath, preCommitted,
			"seed existing values.yaml")).To(Succeed())

		By("creating the RemoteUser & RemoteUserBinding for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer scoped to Helm release secrets with ResourceFinder enabled")
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test34-resource-finder",
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
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("writing the per-spec values override file")
		overridePath := filepath.Join(GinkgoT().TempDir(), "values.yaml")
		Expect(os.WriteFile(overridePath, []byte(
			"replicaCount: 3\ndata:\n  greeting: updated\n"), 0o600)).To(Succeed())

		By("installing the dummy chart as Developer against envtest")
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

		By("waiting for the custom-path values file to be rewritten with the new override")
		Eventually(func(g Gomega) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", customPath)
			g.Expect(err).NotTo(HaveOccurred(),
				"expected file %q on main before uninstall", customPath)
			g.Expect(string(content)).To(ContainSubstring("greeting: updated"),
				"file at %q must reflect the new helm-install override before uninstall", customPath)
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())

		By("uninstalling the dummy chart as Developer")
		Expect(testutils.UninstallChart(chart, actionCfg, settings)).To(Succeed())

		By("checking that the custom-path values file has been removed from the repo")
		Eventually(func(g Gomega) {
			exists, err := fx.Git.FileExists(fx.Repo, "main", customPath)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(exists).To(BeFalse(),
				"expected values file at %q to be removed from main after uninstall", customPath)
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())
	})
})
