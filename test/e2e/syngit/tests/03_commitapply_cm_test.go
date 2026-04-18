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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("03 CommitApply a ConfigMap", func() {

	It("creates the resource on git & cluster, then deletes it from both", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the RemoteUser & RemoteUserBinding for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the CommitApply RemoteSyncer")
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test3",
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: "main",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            fx.RepoURL(),
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{admissionv1.Create, admissionv1.Delete},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"configmaps"},
						},
					}},
				},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("creating a test ConfigMap")
		cm := CreateConfigMap(ctx, fx, "test-cm3", map[string]string{"test": "oui"})

		By("checking the ConfigMap is present on the repo")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("checking the ConfigMap is present on the cluster")
		nnCm := types.NamespacedName{Name: "test-cm3", Namespace: fx.Namespace}
		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx, nnCm, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("deleting the test ConfigMap")
		Eventually(func() error {
			return fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Delete(ctx, "test-cm3", metav1.DeleteOptions{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("checking the ConfigMap is no longer on the repo")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeFalse())

		By("checking the ConfigMap is gone from the cluster")
		Eventually(func() bool {
			err := fx.Users.CtrlAs(utils.Developer).Get(ctx, nnCm, &corev1.ConfigMap{})
			return err != nil && strings.Contains(err.Error(), utils.NotFoundMessage)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})
})
