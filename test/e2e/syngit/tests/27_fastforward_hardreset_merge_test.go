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
)

var _ = Describe("27 Test fast-forward-or-hard-reset merge (TryFastForwardOrHardReset)", func() {

	It("falls back to hard-reset when fast-forward is not possible", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		const upstream = "main"
		const custom = "custom-branch27"

		By("creating the RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())

		By("creating a RemoteTarget for the custom branch with TryFastForwardOrHardReset")
		rtCustom := BuildBranchRemoteTarget(fx, "remotetarget-test27-custom",
			upstream, custom, syngit.TryFastForwardOrHardReset)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rtCustom)).To(Succeed())

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildBranchRUB(fx, "remoteuserbinding-developer",
				"remoteuser-developer", "remotetarget-test27-custom"))).To(Succeed())

		By("creating a RemoteSyncer selecting the custom branch")
		rs1 := BuildBranchRS(fx, "remotesyncer-test27-custom", upstream, custom)
		rs1Copy := rs1.DeepCopy()
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs1)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test27-custom")

		cm1 := CreateConfigMap(ctx, fx, "test-cm27-1", map[string]string{"test": "oui"})
		ExpectOnBranch(fx, custom, cm1)
		ExpectOnCluster(ctx, fx, "test-cm27-1")

		By("deleting the first RemoteSyncer")
		Expect(fx.Users.CtrlAs(utils.Developer).Delete(ctx, rs1)).To(Succeed())

		By("creating RemoteTarget + RemoteSyncer for upstream, so a divergent commit goes there")
		rtUpstream := BuildBranchRemoteTarget(fx, "remotetarget-test27-upstream",
			upstream, upstream, "")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rtUpstream)).To(Succeed())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildBranchRUB(fx, "remoteuserbinding-developer",
				"remoteuser-developer", "remotetarget-test27-upstream", "remotetarget-test27-custom"))).To(Succeed())

		rs2 := BuildBranchRS(fx, "remotesyncer-test27-upstream", upstream, upstream)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs2)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test27-upstream")

		cm2 := CreateConfigMap(ctx, fx, "test-cm27-2", map[string]string{"test": "non"})
		ExpectOnBranch(fx, upstream, cm2)
		ExpectOnCluster(ctx, fx, "test-cm27-2")

		By("deleting the upstream-targeted RemoteSyncer; re-create the custom one")
		Expect(fx.Users.CtrlAs(utils.Developer).Delete(ctx, rs2)).To(Succeed())
		fx.WaitForDynamicWebhookToBeRemoved("remotesyncer-test27-upstream")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs1Copy)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test27-custom")

		By("pushing a 3rd ConfigMap; fast-forward is not possible so the controller hard-resets")
		cm3 := CreateConfigMap(ctx, fx, "test-cm27-3", map[string]string{"test": "non"})

		By("cm1 was only on custom: hard-reset dropped it")
		ExpectNotOnBranch(fx, custom, cm1)
		ExpectOnBranch(fx, custom, cm2)
		ExpectOnBranch(fx, custom, cm3)
		ExpectOnCluster(ctx, fx, "test-cm27-3")
	})
})
