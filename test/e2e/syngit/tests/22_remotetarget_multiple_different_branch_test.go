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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	multiBranch1 = "different-branch1"
	multiBranch2 = "different-branch2"
	multiBranch3 = "different-branch3"
)

var _ = Describe("22 RemoteTarget multiple different branches", func() {

	It("pushes the ConfigMap to every targeted branch including the user-specific one", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		branches := []string{multiBranch1, multiBranch2, multiBranch3}
		rs := BuildBranchRemoteSyncer(fx, "remotesyncer-test22-1",
			map[string]string{
				syngit.RtAnnotationKeyUserSpecific:      string(syngit.RtAnnotationValueOneUserOneBranch),
				syngit.RtAnnotationKeyOneOrManyBranches: strings.Join(branches, ","),
			},
			syngit.MultipleTarget)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("waiting for all RemoteTargets to be associated to the managed RUB")
		Eventually(func() bool {
			rubList := &syngit.RemoteUserBindingList{}
			if err := fx.Users.CtrlAs(utils.Developer).List(ctx, rubList,
				client.InNamespace(fx.Namespace)); err != nil {
				return false
			}
			for _, rub := range rubList.Items {
				if rub.Labels[syngit.ManagedByLabelKey] == syngit.ManagedByLabelValue {
					// Expect 4 targets: developer + branch1 + branch2 + branch3
					return len(rub.Spec.RemoteTargetRefs) >= 4
				}
			}
			return false
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		cm := CreateConfigMap(ctx, fx, "test-cm22-1", map[string]string{"test": "oui"})

		By("ConfigMap should be on the user-specific branch")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, string(utils.Developer), cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("ConfigMap should be on every custom branch")
		for _, br := range branches {
			branch := br
			Eventually(func() (bool, error) {
				return fx.Git.IsObjectInRepo(fx.Repo, branch, cm)
			}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
		}

		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm22-1", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})

	It("denies when multi-branch annotation is used with OneTarget strategy", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		branches := []string{multiBranch1, multiBranch2, multiBranch3}
		rs := BuildBranchRemoteSyncer(fx, "remotesyncer-test22-2",
			map[string]string{
				syngit.RtAnnotationKeyUserSpecific:      string(syngit.RtAnnotationValueOneUserOneBranch),
				syngit.RtAnnotationKeyOneOrManyBranches: strings.Join(branches, ","),
			},
			syngit.OneTarget)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		cm := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm22-2", Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err != nil && syngiterrors.Is(err, syngiterrors.ErrTooMuchRemoteTarget)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})
})
