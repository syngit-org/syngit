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
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	selTestLabelKey   = "my-label-key"
	selTestLabelValue = "my-label-value"
)

func makeSelectorRemoteSyncer(fx *utils.Fixture, name string, selector *metav1.LabelSelector) *syngit.RemoteSyncer {
	return &syngit.RemoteSyncer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
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
			RemoteUserBindingSelector:   selector,
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
}

// makeSelectorRUB builds a RUB with labels, referencing the auto-generated
// controller-side RemoteTarget and the named RemoteUser.
func makeSelectorRUB(fx *utils.Fixture, name, ruName string) *syngit.RemoteUserBinding {
	rtName := fmt.Sprintf("%s-%s-main-%s-%s-main",
		fx.Repo.Owner, fx.Repo.Name, fx.Repo.Owner, fx.Repo.Name)
	return &syngit.RemoteUserBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fx.Namespace,
			Labels:    map[string]string{selTestLabelKey: selTestLabelValue},
		},
		Spec: syngit.RemoteUserBindingSpec{
			RemoteUserRefs:   []corev1.ObjectReference{{Name: ruName}},
			RemoteTargetRefs: []corev1.ObjectReference{{Name: rtName}},
			Subject:          rbacv1.Subject{Kind: "User", Name: string(utils.Developer)},
		},
	}
}

var _ = Describe("17 RemoteUserBinding selector in RemoteSyncer", func() {

	It("denies when the selector matches no RemoteUserBinding", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			makeSelectorRUB(fx, "custom-rub-for-developer", "remoteuser-developer"))).To(Succeed())

		mismatch := &metav1.LabelSelector{MatchLabels: map[string]string{selTestLabelKey: "another-value"}}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			makeSelectorRemoteSyncer(fx, "remotesyncer-test17-1", mismatch))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test17-1")

		cm := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm17-1", Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err != nil && syngiterrors.Is(err, syngiterrors.ErrRemoteUserBindingNotFound)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("the ConfigMap is not on cluster")
		Eventually(func() bool {
			err := fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm17-1", Namespace: fx.Namespace}, &corev1.ConfigMap{})
			return err != nil && strings.Contains(err.Error(), utils.NotFoundMessage)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})

	It("pushes when the selector matches a labeled RemoteUserBinding", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			makeSelectorRUB(fx, "custom-rub-for-developer", "remoteuser-developer"))).To(Succeed())

		match := &metav1.LabelSelector{MatchLabels: map[string]string{selTestLabelKey: selTestLabelValue}}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			makeSelectorRemoteSyncer(fx, "remotesyncer-test17-2", match))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test17-2")

		cm := CreateConfigMap(ctx, fx, "test-cm17-2", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm17-2", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})

	It("pushes when no selector is specified (selector defaults to all)", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			makeSelectorRUB(fx, "custom-rub-for-developer", "remoteuser-developer"))).To(Succeed())

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			makeSelectorRemoteSyncer(fx, "remotesyncer-test17-3", nil))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test17-3")

		cm := CreateConfigMap(ctx, fx, "test-cm17-3", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm17-3", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})
})
