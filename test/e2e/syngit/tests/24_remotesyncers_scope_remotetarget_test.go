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

	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("24 One RemoteTarget scoped by multiple RemoteSyncers", func() {

	It("allows pushing again after the second RemoteSyncer releases the shared RemoteTarget & RUB", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		rs1 := BuildDefaultCmRemoteSyncer("remotesyncer-test24-1", fx.Namespace, "main", fx.RepoURL())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs1)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test24-1")

		rs2 := BuildDefaultCmRemoteSyncer("remotesyncer-test24-2", fx.Namespace, "main", fx.RepoURL())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs2)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test24-2")

		By("deleting the first RemoteSyncer to release the scoped RemoteTarget & RUB")
		Expect(fx.Users.CtrlAs(utils.Developer).Delete(ctx, rs1)).To(Succeed())
		fx.WaitForDynamicWebhookToBeRemoved("remotesyncer-test24-1")

		cm := CreateConfigMap(ctx, fx, "test-cm24", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm24", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})
})
