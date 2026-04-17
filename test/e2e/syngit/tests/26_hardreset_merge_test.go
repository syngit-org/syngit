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

var _ = Describe("26 Test hard-reset merge (TryHardResetOrDie)", func() {

	// runMergeScenario isolates the differences between the two sub-cases.
	// doExternalMerge decides whether an external merge from custom to
	// upstream is performed before the second custom-branch push.
	runMergeScenario := func(suffix string, doExternalMerge bool) {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		const upstream = "main"
		const custom = "custom-branch26"

		rtCustom := "remotetarget-test26-custom-" + suffix
		rtUpstream := "remotetarget-test26-upstream-" + suffix
		rs1Name := "remotesyncer-test26-custom-" + suffix
		rs2Name := "remotesyncer-test26-upstream-" + suffix
		cm1Name := "test-cm26-1-" + suffix
		cm2Name := "test-cm26-2-" + suffix
		cm3Name := "test-cm26-3-" + suffix

		By("creating the RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())

		rt := BuildBranchRemoteTarget(fx, rtCustom, upstream, custom, syngit.TryHardResetOrDie)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rt)).To(Succeed())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildBranchRUB(fx, "remoteuserbinding-developer", "remoteuser-developer", rtCustom))).To(Succeed())

		rs1 := BuildBranchRS(fx, rs1Name, upstream, custom)
		rs1DeepCopy := rs1.DeepCopy()
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs1)).To(Succeed())
		fx.WaitForDynamicWebhook(rs1Name)

		cm1 := CreateConfigMap(ctx, fx, cm1Name, map[string]string{"test": "oui"})
		ExpectOnBranch(fx, custom, cm1)
		ExpectOnCluster(ctx, fx, cm1Name)

		Expect(fx.Users.CtrlAs(utils.Developer).Delete(ctx, rs1)).To(Succeed())

		rtUp := BuildBranchRemoteTarget(fx, rtUpstream, upstream, upstream, "")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rtUp)).To(Succeed())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildBranchRUB(fx, "remoteuserbinding-developer",
				"remoteuser-developer", rtUpstream, rtCustom))).To(Succeed())

		rs2 := BuildBranchRS(fx, rs2Name, upstream, upstream)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs2)).To(Succeed())
		fx.WaitForDynamicWebhook(rs2Name)

		cm2 := CreateConfigMap(ctx, fx, cm2Name, map[string]string{"test": "non"})
		ExpectOnBranch(fx, upstream, cm2)
		ExpectOnCluster(ctx, fx, cm2Name)

		if doExternalMerge {
			Expect(fx.Git.MergeBranch(fx.Repo, custom, upstream)).To(Succeed())
		}

		Expect(fx.Users.CtrlAs(utils.Developer).Delete(ctx, rs2)).To(Succeed())
		fx.WaitForDynamicWebhookToBeRemoved(rs2Name)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs1DeepCopy)).To(Succeed())
		fx.WaitForDynamicWebhook(rs1Name)

		cm3 := CreateConfigMap(ctx, fx, cm3Name, map[string]string{"test": "non"})

		if doExternalMerge {
			By("external merge happened: cm1 is preserved")
			ExpectOnBranch(fx, custom, cm1)
		} else {
			By("no external merge: hard-reset discards cm1 (was present only on custom)")
			ExpectNotOnBranch(fx, custom, cm1)
		}
		ExpectOnBranch(fx, custom, cm2)
		ExpectOnBranch(fx, custom, cm3)
		ExpectOnCluster(ctx, fx, cm3Name)
	}

	It("preserves upstream and custom content when an external merge happened", func() {
		runMergeScenario("merged", true)
	})

	It("overwrites the custom branch when no external merge happened", func() {
		runMergeScenario("unmerged", false)
	})
})
