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
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"

	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
)

var _ = Describe("25 Test fast-forward merge (TryFastForwardOrDie)", func() {

	It("pulls upstream changes into the custom branch on subsequent pushes", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		const upstream = "main"
		const custom = "custom-branch25"

		By("creating the RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())

		By("creating a RemoteTarget for the custom branch with TryFastForwardOrDie")
		rtCustom := BuildBranchRemoteTarget(fx, "remotetarget-test25-1",
			upstream, custom, syngit.TryFastForwardOrDie)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rtCustom)).To(Succeed())

		By("creating the RUB referencing the custom-branch RemoteTarget")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildBranchRUB(fx, "remoteuserbinding-developer",
				"remoteuser-developer", "remotetarget-test25-1"))).To(Succeed())

		By("creating the RemoteSyncer selecting the custom branch")
		rs1 := BuildBranchRS(fx, "remotesyncer-test25-1", upstream, custom)
		rs1DeepCopy := rs1.DeepCopy()
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs1)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test25-1")

		By("creating a ConfigMap routed to the custom branch")
		cm1 := CreateConfigMap(ctx, fx, "test-cm25-1", map[string]string{"test": "oui"})
		ExpectOnBranch(fx, custom, cm1)
		ExpectOnCluster(ctx, fx, "test-cm25-1")

		By("deleting the first RemoteSyncer")
		Expect(fx.Users.CtrlAs(utils.Developer).Delete(ctx, rs1)).To(Succeed())

		By("creating a RemoteTarget for the upstream branch")
		rtUpstream := BuildBranchRemoteTarget(fx, "remotetarget-test25-2",
			upstream, upstream, "")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rtUpstream)).To(Succeed())

		By("updating the RUB to reference both RemoteTargets")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildBranchRUB(fx, "remoteuserbinding-developer",
				"remoteuser-developer", "remotetarget-test25-2", "remotetarget-test25-1"))).To(Succeed())

		By("creating the RemoteSyncer selecting the upstream branch")
		rs2 := BuildBranchRS(fx, "remotesyncer-test25-2", upstream, upstream)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs2)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test25-2")

		By("pushing a ConfigMap to the upstream branch")
		cm2 := CreateConfigMap(ctx, fx, "test-cm25-2", map[string]string{"test": "non"})
		ExpectOnBranch(fx, upstream, cm2)
		ExpectOnCluster(ctx, fx, "test-cm25-2")

		By("simulating an external merge from custom to upstream")
		Expect(fx.Git.MergeBranch(fx.Repo, custom, upstream)).To(Succeed())

		By("deleting the second RemoteSyncer and re-creating the first")
		Expect(fx.Users.CtrlAs(utils.Developer).Delete(ctx, rs2)).To(Succeed())
		fx.WaitForDynamicWebhookToBeRemoved("remotesyncer-test25-2")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs1DeepCopy)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test25-1")

		By("pushing another ConfigMap - controller should fast-forward pull from upstream first")
		cm3 := CreateConfigMap(ctx, fx, "test-cm25-3", map[string]string{"test": "non"})

		By("all 3 ConfigMaps should be present on the custom branch")
		ExpectOnBranch(fx, custom, cm1)
		ExpectOnBranch(fx, custom, cm2)
		ExpectOnBranch(fx, custom, cm3)
		ExpectOnCluster(ctx, fx, "test-cm25-3")
	})
})
