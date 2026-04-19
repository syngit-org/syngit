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
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("29 Add & remove patterns tests", func() {

	It("pushes to the right branches as RemoteSyncer annotations change", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const (
			branch1 = "branch29-1"
			branch2 = "branch29-2"
		)

		By("creating the managed RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer with both user-specific and explicit-branches annotations")
		rs := BuildBranchRemoteSyncer(fx, "remotesyncer-test29",
			map[string]string{
				syngit.RtAnnotationKeyOneOrManyBranches: strings.Join([]string{branch1, branch2}, ", "),
				syngit.RtAnnotationKeyUserSpecific:      string(syngit.RtAnnotationValueOneUserOneBranch),
			},
			syngit.MultipleTarget)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test29")

		By("waiting for the controller to populate the managed RUB with >=3 targets")
		Eventually(func() bool {
			rubList := &syngit.RemoteUserBindingList{}
			if err := fx.Users.CtrlAs(utils.Developer).List(ctx, rubList,
				client.InNamespace(fx.Namespace)); err != nil {
				return false
			}
			for _, rub := range rubList.Items {
				if rub.Labels[syngit.ManagedByLabelKey] == syngit.ManagedByLabelValue {
					return len(rub.Spec.RemoteTargetRefs) >= 3
				}
			}
			return false
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("creating a test configmap that should be push everywhere")
		cm1 := CreateConfigMap(ctx, fx, "test-cm29-1", map[string]string{"test": "oui"})
		for _, br := range []string{branch1, branch2, string(utils.Developer)} {
			ExpectOnBranch(fx, br, cm1)
		}

		By("checking that the configmap is present on the branches")
		ExpectOnBranch(fx, branch1, cm1)
		ExpectOnBranch(fx, branch2, cm1)
		ExpectOnBranch(fx, string(utils.Developer), cm1)

		By("removing explicit-branches so only the user-specific target remains")
		rs.Annotations[syngit.RtAnnotationKeyOneOrManyBranches] = ""
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test29")

		By("waiting for the branch targets to be cleaned up from the managed RUB")
		Eventually(func() bool {
			rubList := &syngit.RemoteUserBindingList{}
			if err := fx.Users.CtrlAs(utils.Developer).List(ctx, rubList,
				client.InNamespace(fx.Namespace)); err != nil {
				return false
			}
			for _, rub := range rubList.Items {
				if rub.Labels[syngit.ManagedByLabelKey] == syngit.ManagedByLabelValue {
					return len(rub.Spec.RemoteTargetRefs) == 1
				}
			}
			return false
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		cm2 := CreateConfigMap(ctx, fx, "test-cm29-2", map[string]string{"test": "oui"})
		ExpectNotOnBranch(fx, branch1, cm2)
		ExpectNotOnBranch(fx, branch2, cm2)
		ExpectOnBranch(fx, string(utils.Developer), cm2)

		By("removing the user-specific annotation too - push should fail")
		rs.Annotations[syngit.RtAnnotationKeyUserSpecific] = ""
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test29")

		cm3 := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm29-3", Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm3, metav1.CreateOptions{})
			return err != nil && syngiterrors.Is(err, syngiterrors.ErrRemoteTargetNotFound)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		for _, br := range []string{branch1, branch2, string(utils.Developer)} {
			ExpectNotOnBranch(fx, br, cm3)
		}
	})

	It("removes the user-specific push target when the RemoteUser becomes unmanaged", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the managed RemoteUser for Developer")
		ruDev := fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, ruDev)).To(Succeed())

		By("creating the RemoteSyncer with only the user-specific annotation")
		rs := BuildBranchRemoteSyncer(fx, "remotesyncer-test29-user",
			map[string]string{
				syngit.RtAnnotationKeyUserSpecific: string(syngit.RtAnnotationValueOneUserOneBranch),
			},
			syngit.MultipleTarget)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test29-user")

		cm1 := CreateConfigMap(ctx, fx, "test-cm29-4", map[string]string{"test": "oui"})
		ExpectOnBranch(fx, string(utils.Developer), cm1)

		By("flipping the managed annotation to false on the RemoteUser")
		ruDev.Annotations[syngit.RubAnnotationKeyManaged] = "false"
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, ruDev)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test29-user")

		cm2 := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm29-5", Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm2, metav1.CreateOptions{})
			return err != nil && syngiterrors.Is(err, syngiterrors.ErrRemoteUserNotFound)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		ExpectNotOnBranch(fx, string(utils.Developer), cm2)
	})
})
