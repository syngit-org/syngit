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
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("05 Use a default user", func() {

	It("falls back to the RemoteSyncer default user when the actor has no mapping", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating a bogus-creds secret representing a user with no git access")
		bogus := fx.NewBogusCredsSecret("bogus-creds")
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx, bogus)).To(Succeed())

		By("creating RemoteUser pointing at bogus creds (push will fail)")
		ruBogus := fx.NewRemoteUser(utils.Developer, "remoteuser-bogus", false)
		ruBogus.Spec.SecretRef.Name = "bogus-creds"
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, ruBogus)).To(Succeed())

		By("creating RemoteUser for Developer with valid creds (push will succeed)")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())

		By("creating the default RemoteTarget")
		rt := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{Name: "remotetarget-test5", Namespace: fx.Namespace},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: fx.RepoURL(),
				TargetRepository:   fx.RepoURL(),
				UpstreamBranch:     "main",
				TargetBranch:       "main",
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rt)).To(Succeed())

		By("creating the RemoteSyncer using the bogus RemoteUser as default")
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test5",
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: "main",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.UseDefaultUser,
				ExcludedFields:              []string{".metadata.uid"},
				DefaultRemoteUserRef:        &corev1.ObjectReference{Name: "remoteuser-bogus"},
				DefaultRemoteTargetRef:      &corev1.ObjectReference{Name: "remotetarget-test5"},
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
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Admin, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("Admin tries to create a ConfigMap - push fails because default user has bogus creds")
		cm := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm5", Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Admin).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
				// The error could be different depending on the git platform.
			return err != nil && (strings.Contains(err.Error(), "authentication required") ||
				strings.Contains(err.Error(), "unauthorized") ||
				strings.Contains(err.Error(), "permission denied"))
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("updating the RemoteSyncer to use Developer's valid RemoteUser as default")
		rs.Spec.DefaultRemoteUserRef.Name = "remoteuser-developer"
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Admin, rs)).To(Succeed())
		Eventually(func() bool {
			getRemoteSyncer := &syngit.RemoteSyncer{}
			err := fx.Users.CtrlAs(utils.Developer).Get(fx.Ctx,
				types.NamespacedName{Name: rs.Name, Namespace: rs.Namespace}, getRemoteSyncer)
			if err != nil {
				return false
			}
			return getRemoteSyncer.Spec.DefaultRemoteUserRef.Name == rs.Spec.DefaultRemoteUserRef.Name
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("the push should succeed with the valid fallback user")
		Eventually(func() error {
			_, err := fx.Users.KAs(utils.Admin).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("the ConfigMap is present in the repo")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("the ConfigMap is present on the cluster")
		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm5", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})
})
