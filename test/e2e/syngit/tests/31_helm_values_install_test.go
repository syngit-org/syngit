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

var _ = Describe("31 HelmValuesMutation install commits the values file", func() {

	It("commits a values.yaml whose first line is the resource-finder marker (first install on empty repo)", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const (
			releaseName = "demo"
			markerLine  = "# syngit.resource-finder/v1: "
		)

		By("creating the RemoteUser & RemoteUserBinding for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer scoped to Helm release secrets")
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test31-first-install",
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
				// ResourceFinder enabled but will not work since the chart
				// values has never been committed & pushed before.
				ResourceFinder: true,
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

		By("checking the values file landed at <ns>/chart-values/<name>.yaml with the marker first line")
		secretName := "sh.helm.release.v1." + releaseName + ".v1"
		expectedMarker := markerLine + fx.Namespace + "/" + secretName + "\n"
		expectedPath := fmt.Sprintf("%s/%s/%s.yaml", fx.Namespace, mutator.DefaultChartValuesSubPath, secretName)
		Eventually(func(g Gomega) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", expectedPath)
			g.Expect(err).NotTo(HaveOccurred(),
				"expected committed file at %q on main", expectedPath)
			g.Expect(string(content)).To(HavePrefix(expectedMarker),
				"committed values file must start with the resource-finder marker")
			g.Expect(string(content)).To(ContainSubstring("greeting: hello"),
				"committed values file must contain the user-supplied override")
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())
	})

	It("rewrites the existing values file in place at its custom path (update via ResourceFinder)", func() {
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
				Name:      "remotesyncer-test31-resource-finder",
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

		By("checking the values file was rewritten in place at the custom path")
		expectedMarker := preCommittedMarker + "\n"
		Eventually(func(g Gomega) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", customPath)
			g.Expect(err).NotTo(HaveOccurred(),
				"expected pre-committed file %q to still exist on main", customPath)
			g.Expect(string(content)).To(HavePrefix(expectedMarker),
				"file at %q must start with the resource-finder output marker", customPath)
			g.Expect(string(content)).To(ContainSubstring("greeting: updated"),
				"file at %q must reflect the new helm-install override", customPath)
			g.Expect(string(content)).NotTo(ContainSubstring("greeting: previous"),
				"file at %q must no longer contain the pre-seeded value", customPath)
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())

		By("checking no new file was created at the default chart-values path")
		defaultPath := fmt.Sprintf("%s/%s/%s.yaml", fx.Namespace, mutator.DefaultChartValuesSubPath, secretName)
		exists, err := fx.Git.FileExists(fx.Repo, "main", defaultPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeFalse(),
			"ResourceFinder matched the custom path, so no fallback file at %q should be created",
			defaultPath)
	})

	It("commits values that the user overrode and omits chart defaults", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const (
			releaseName = "demo"
			markerLine  = "# syngit.resource-finder/v1: "
		)

		By("creating the RemoteUser & RemoteUserBinding for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer scoped to Helm release secrets")
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test31-user-supplied",
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
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("writing a values override that overrides `data` but leaves `replicaCount` to the chart default")
		overridePath := filepath.Join(GinkgoT().TempDir(), "values.yaml")
		Expect(os.WriteFile(overridePath, []byte(
			"data:\n  greeting: hello\n"), 0o600)).To(Succeed())

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

		By("checking the committed values file holds only the user-supplied override")
		secretName := "sh.helm.release.v1." + releaseName + ".v1"
		expectedMarker := markerLine + fx.Namespace + "/" + secretName + "\n"
		expectedPath := fmt.Sprintf("%s/%s/%s.yaml", fx.Namespace, mutator.DefaultChartValuesSubPath, secretName)
		Eventually(func(g Gomega) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", expectedPath)
			g.Expect(err).NotTo(HaveOccurred(),
				"expected committed file at %q on main", expectedPath)
			g.Expect(string(content)).To(HavePrefix(expectedMarker),
				"committed values file must start with the resource-finder marker")
			g.Expect(string(content)).To(ContainSubstring("greeting: hello"),
				"committed values file must contain the user-supplied override")
			g.Expect(string(content)).NotTo(ContainSubstring("replicaCount"),
				"committed values file must not include chart defaults the user did not override")
		}).WithTimeout(utils.DefaultTimeout).
			WithPolling(utils.DefaultInterval).
			Should(Succeed())
	})
})
