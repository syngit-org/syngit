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
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("08 Webhook RBAC checker", func() {

	It("denies creating a RemoteSyncer scoped to resources the user lacks permission on", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the managed RemoteUser for Restricted")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted,
			fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", true))).To(Succeed())

		By("Restricted tries to create a RemoteSyncer scoping configmaps (beyond its RBAC)")
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test8-1",
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
		err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, rs)
		Expect(err).To(HaveOccurred())
		Expect(syngiterrors.Is(err, syngiterrors.ErrResourceScopeForbidden)).To(BeTrue())
	})

	It("allows creating a RemoteSyncer whose scope matches Restricted's RBAC", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the managed RemoteUser for Restricted")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted,
			fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", true))).To(Succeed())

		By("Restricted tries a RemoteSyncer for secrets including DELETE (lacks delete permission)")
		rsBad := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test8-2-bad",
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
							Resources:   []string{"secrets"},
						},
					}},
				},
			},
		}
		err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, rsBad)
		Expect(err).To(HaveOccurred())
		Expect(syngiterrors.Is(err, syngiterrors.ErrResourceScopeForbidden)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("DELETE"))

		By("Restricted creates a RemoteSyncer with scope matching its RBAC (create secrets only)")
		rsOK := rsBad.DeepCopy()
		rsOK.Name = "remotesyncer-test8-2-ok"
		rsOK.Spec.ScopedResources.Rules[0].Operations = []admissionv1.OperationType{admissionv1.Create}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted, rsOK)).To(Succeed())
		fx.WaitForDynamicWebhook(rsOK.Name)

		By("creating a test secret")
		sec := &corev1.Secret{
			TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-secret8", Namespace: fx.Namespace},
			StringData: map[string]string{"test": "test1"},
		}
		Eventually(func() error {
			_, err := fx.Users.KAs(utils.Restricted).CoreV1().Secrets(fx.Namespace).
				Create(ctx, sec, metav1.CreateOptions{})
			return err
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("the secret is present in the repo")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", sec)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("the secret is present on the cluster")
		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Admin).Get(ctx,
				types.NamespacedName{Name: "test-secret8", Namespace: fx.Namespace}, &corev1.Secret{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})
})
