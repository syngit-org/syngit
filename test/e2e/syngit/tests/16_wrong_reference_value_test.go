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
	"strings"

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

var _ = Describe("16 Wrong reference or value test", func() {

	baseCMRS := func(fx *utils.Fixture, mode syngit.DefaultUnauthorizedUserMode,
		defaultRU, defaultRT string) *syngit.RemoteSyncer {
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test16",
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: "main",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: mode,
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
		if defaultRU != "" {
			rs.Spec.DefaultRemoteUserRef = &corev1.ObjectReference{Name: defaultRU}
		}
		if defaultRT != "" {
			rs.Spec.DefaultRemoteTargetRef = &corev1.ObjectReference{Name: defaultRT}
		}
		return rs
	}

	It("errors when objects reference missing secrets / RUs / RTs", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating a RemoteUser with a fake secret ref")
		ruBad := fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false)
		ruBad.Spec.SecretRef.Name = "fake-secret"
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, ruBad)).To(Succeed())

		By("checking the RemoteUser reports SecretBound=False")
		Eventually(func() bool {
			got := &syngit.RemoteUser{}
			err := fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "remoteuser-developer", Namespace: fx.Namespace}, got)
			if err != nil || len(got.Status.Conditions) == 0 {
				return false
			}
			return got.Status.Conditions[0].Type == "SecretBound" &&
				got.Status.Conditions[0].Status == metav1.ConditionFalse
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("creating a RemoteUserBinding referencing non-existent RU and RT")
		rubBad := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuserbinding-developer",
				Namespace: fx.Namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs:   []corev1.ObjectReference{{Name: "fake-remoteuser"}},
				RemoteTargetRefs: []corev1.ObjectReference{{Name: "fake-remotetarget"}},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rubBad)).To(Succeed())

		By("creating a RemoteSyncer in Block mode")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			baseCMRS(fx, syngit.Block, "", ""))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test16")

		cm := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm16", Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}

		By("creating a ConfigMap as Developer -> RemoteUserBinding invalid: error")
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err != nil && syngiterrors.Is(err, syngiterrors.ErrRemoteUserBindingNotFound)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("switching the RemoteSyncer to UseDefaultUser with fake default references")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			baseCMRS(fx, syngit.UseDefaultUser, "fake-defaultuser", "fake-defaulttarget"))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test16")

		By("creating the ConfigMap again -> DefaultRemoteUserNotFoundError")
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err != nil && syngiterrors.Is(err, syngiterrors.ErrRemoteUserNotFound)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("creating a RemoteUser (with a valid secret) for Restricted to use as default user")
		ruRestricted := fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", false)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted, ruRestricted)).To(Succeed())

		By("updating the RemoteSyncer to reference a valid default RU but still-fake default RT")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			baseCMRS(fx, syngit.UseDefaultUser, "remoteuser-restricted", "fake-defaulttarget"))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test16")

		By("creating the ConfigMap -> DefaultRemoteTargetNotFoundError")
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err != nil && syngiterrors.Is(err, syngiterrors.ErrRemoteTargetNotFound)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("no ConfigMap should have reached the cluster")
		nnCm := types.NamespacedName{Name: "test-cm16", Namespace: fx.Namespace}
		Eventually(func() bool {
			err := fx.Users.CtrlAs(utils.Developer).Get(ctx, nnCm, &corev1.ConfigMap{})
			fmt.Println(err)
			return err != nil && strings.Contains(err.Error(), utils.NotFoundMessage)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})
})
