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

	syngitv1beta3 "github.com/syngit-org/syngit/pkg/api/v1beta3"
	syngitv1beta4 "github.com/syngit-org/syngit/pkg/api/v1beta4"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("15 conversion webhook test", func() {

	It("converts old API objects to new API on read", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating a old API RemoteUser")
		ruOld := &syngitv1beta3.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuser-developer",
				Namespace: fx.Namespace,
			},
			Spec: syngitv1beta3.RemoteUserSpec{
				Email:             utils.DefaultEmail(utils.Developer),
				GitBaseDomainFQDN: fx.FQDN(),
				SecretRef:         corev1.SecretReference{Name: "developer-creds"},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, ruOld)).To(Succeed())

		By("reading the RemoteUser as new API should succeed (conversion)")
		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "remoteuser-developer", Namespace: fx.Namespace},
				&syngitv1beta4.RemoteUser{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("creating a old API RemoteUserBinding")
		rubOld := &syngitv1beta3.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuserbinding-developer",
				Namespace: fx.Namespace,
			},
			Spec: syngitv1beta3.RemoteUserBindingSpec{
				RemoteUserRefs: []corev1.ObjectReference{{Name: "fake-remoteuser"}},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rubOld)).To(Succeed())

		By("reading the RemoteUserBinding as new API should succeed (conversion)")
		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "remoteuserbinding-developer", Namespace: fx.Namespace},
				&syngitv1beta4.RemoteUserBinding{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("creating a old API RemoteSyncer")
		rsOld := &syngitv1beta3.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test15",
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngitv1beta3.RtAnnotationKeyOneOrManyBranches: "main",
				},
			},
			Spec: syngitv1beta3.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBlockAppliedMessage:  utils.DefaultDeniedMessage,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngitv1beta3.Block,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngitv1beta3.CommitOnly,
				TargetStrategy:              syngitv1beta3.OneTarget,
				RemoteRepository:            "https://fake-repo.com/my_repo.git",
				ScopedResources: syngitv1beta3.ScopedResources{
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
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rsOld)).To(Succeed())

		By("reading the RemoteSyncer as new API should succeed (conversion)")
		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "remotesyncer-test15", Namespace: fx.Namespace},
				&syngitv1beta4.RemoteSyncer{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})
})
