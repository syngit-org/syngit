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
	syngitutils "github.com/syngit-org/syngit/pkg/utils"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	rtSelLabelKey   = "my-label-key"
	rtSelLabelValue = "my-label-value"
)

var _ = Describe("23 RemoteTarget selector in RemoteSyncer", func() {

	It("denies when the RemoteTargetSelector matches no RemoteTarget", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())

		rt := BuildDefaultRemoteTarget(fx, "remotetarget-test23-1", "main",
			map[string]string{rtSelLabelKey: rtSelLabelValue})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rt)).To(Succeed())

		rub := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "remoteuserbinding-developer", Namespace: fx.Namespace},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs:   []corev1.ObjectReference{{Name: "remoteuser-developer"}},
				RemoteTargetRefs: []corev1.ObjectReference{{Name: "remotetarget-test23-1"}},
				Subject:          rbacv1.Subject{Kind: "User", Name: string(utils.Developer)},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rub)).To(Succeed())

		mismatch := &metav1.LabelSelector{MatchLabels: map[string]string{rtSelLabelKey: "another-value"}}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildRemoteTargetSelectorRS(fx, "remotesyncer-test23-1", mismatch, syngit.OneTarget, nil))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test23-1")

		cm := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm23-1", Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err != nil && syngitutils.ErrorTypeChecker(&syngitutils.RemoteTargetNotFoundError{}, err.Error())
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		Eventually(func() bool {
			err := fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm23-1", Namespace: fx.Namespace}, &corev1.ConfigMap{})
			return err != nil && strings.Contains(err.Error(), utils.NotFoundMessage)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})

	It("pushes when the RemoteTargetSelector matches a labeled RemoteTarget", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())

		rt := BuildDefaultRemoteTarget(fx, "remotetarget-test23-2", "main",
			map[string]string{rtSelLabelKey: rtSelLabelValue})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rt)).To(Succeed())

		rub := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "remoteuserbinding-developer", Namespace: fx.Namespace},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs:   []corev1.ObjectReference{{Name: "remoteuser-developer"}},
				RemoteTargetRefs: []corev1.ObjectReference{{Name: "remotetarget-test23-2"}},
				Subject:          rbacv1.Subject{Kind: "User", Name: string(utils.Developer)},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rub)).To(Succeed())

		match := &metav1.LabelSelector{MatchLabels: map[string]string{rtSelLabelKey: rtSelLabelValue}}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildRemoteTargetSelectorRS(fx, "remotesyncer-test23-2", match, syngit.OneTarget, nil))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test23-2")

		cm := CreateConfigMap(ctx, fx, "test-cm23-2", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm23-2", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})

	It("pushes when no RemoteTargetSelector is specified", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())

		rt := BuildDefaultRemoteTarget(fx, "remotetarget-test23-3", "main",
			map[string]string{rtSelLabelKey: rtSelLabelValue})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rt)).To(Succeed())

		rub := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "remoteuserbinding-developer", Namespace: fx.Namespace},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs:   []corev1.ObjectReference{{Name: "remoteuser-developer"}},
				RemoteTargetRefs: []corev1.ObjectReference{{Name: "remotetarget-test23-3"}},
				Subject:          rbacv1.Subject{Kind: "User", Name: string(utils.Developer)},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rub)).To(Succeed())

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildRemoteTargetSelectorRS(fx, "remotesyncer-test23-3", nil, syngit.OneTarget, nil))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test23-3")

		cm := CreateConfigMap(ctx, fx, "test-cm23-3", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})

	It("works with multiple RemoteTargets selected", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const (
			branch1 = "branch23-31"
			branch2 = "branch23-32"
		)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())

		rt1 := BuildDefaultRemoteTarget(fx, "remotetarget-test23-41", branch1,
			map[string]string{rtSelLabelKey: rtSelLabelValue})
		rt1.Spec.MergeStrategy = syngit.TryFastForwardOrDie
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rt1)).To(Succeed())

		rt2 := BuildDefaultRemoteTarget(fx, "remotetarget-test23-42", branch2,
			map[string]string{rtSelLabelKey: rtSelLabelValue})
		rt2.Spec.MergeStrategy = syngit.TryFastForwardOrDie
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rt2)).To(Succeed())

		rub := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "remoteuserbinding-developer", Namespace: fx.Namespace},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs: []corev1.ObjectReference{{Name: "remoteuser-developer"}},
				RemoteTargetRefs: []corev1.ObjectReference{
					{Name: "remotetarget-test23-41"},
					{Name: "remotetarget-test23-42"},
				},
				Subject: rbacv1.Subject{Kind: "User", Name: string(utils.Developer)},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rub)).To(Succeed())

		match := &metav1.LabelSelector{MatchLabels: map[string]string{rtSelLabelKey: rtSelLabelValue}}
		annotations := map[string]string{syngit.RtAnnotationKeyOneOrManyBranches: "main"}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildRemoteTargetSelectorRS(fx, "remotesyncer-test23-4", match, syngit.MultipleTarget, annotations))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test23-4")

		cm := CreateConfigMap(ctx, fx, "test-cm23-4", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, branch1, cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, branch2, cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})

	It("denies when the matched RemoteTarget is not part of the user's RUB", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		rt := BuildDefaultRemoteTarget(fx, "remotetarget-test23-5", "main",
			map[string]string{rtSelLabelKey: rtSelLabelValue})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rt)).To(Succeed())

		match := &metav1.LabelSelector{MatchLabels: map[string]string{rtSelLabelKey: rtSelLabelValue}}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildRemoteTargetSelectorRS(fx, "remotesyncer-test23-5", match, syngit.OneTarget, nil))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test23-5")

		cm := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm23-5", Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err != nil && syngitutils.ErrorTypeChecker(&syngitutils.RemoteTargetNotFoundError{}, err.Error())
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})
})
