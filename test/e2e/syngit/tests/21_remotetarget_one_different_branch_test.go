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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("21 RemoteTarget one different branch", func() {

	It("pushes the ConfigMap to the user-specific branch (OneUserOneBranch)", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		rs := BuildBranchRemoteSyncer(fx, "remotesyncer-test21-1",
			map[string]string{syngit.RtAnnotationKeyUserSpecific: string(syngit.RtAnnotationValueOneUserOneBranch)},
			syngit.OneTarget)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		cm := CreateConfigMap(ctx, fx, "test-cm21-1", map[string]string{"test": "oui"})

		By("the ConfigMap should land on the developer-specific branch")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, string(utils.Developer), cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm21-1", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})

	It("pushes the ConfigMap to a single custom branch via MultipleTarget strategy", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const customBranch = "custom-branch21"

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		rs := BuildBranchRemoteSyncer(fx, "remotesyncer-test21-2",
			map[string]string{syngit.RtAnnotationKeyOneOrManyBranches: customBranch},
			syngit.MultipleTarget)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		cm := CreateConfigMap(ctx, fx, "test-cm21-2", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, customBranch, cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm21-2", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})
})
