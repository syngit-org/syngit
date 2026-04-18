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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("20 All syngit objects without annotations test", func() {

	It("runs the full workflow without relying on syngit annotations", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the RemoteUser for Developer (no annotations)")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())

		By("creating a RemoteTarget with upstream == target")
		rt := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{Name: "remotetarget-test20", Namespace: fx.Namespace},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: fx.RepoURL(),
				TargetRepository:   fx.RepoURL(),
				UpstreamBranch:     "main",
				TargetBranch:       "main",
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rt)).To(Succeed())

		By("creating the RemoteUserBinding manually")
		rub := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "remoteuserbinding-developer", Namespace: fx.Namespace},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs:   []corev1.ObjectReference{{Name: "remoteuser-developer"}},
				RemoteTargetRefs: []corev1.ObjectReference{{Name: "remotetarget-test20"}},
				Subject:          rbacv1.Subject{Kind: "User", Name: string(utils.Developer)},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rub)).To(Succeed())

		By("creating the RemoteSyncer without any annotations")
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{Name: "remotesyncer-test20", Namespace: fx.Namespace},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            fx.RepoURL(),
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{admissionv1.Create},
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
		cm := CreateConfigMap(ctx, fx, "test-cm20", map[string]string{"test": "oui"})

		By("the ConfigMap should land in the repo and on the cluster")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm20", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})
})
