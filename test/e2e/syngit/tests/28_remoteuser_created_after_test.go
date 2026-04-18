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
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("28 RemoteUser created after RemoteSyncer & RemoteTargets", func() {

	It("associates pre-existing managed RemoteTargets when the RemoteUser is created later", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const (
			branch1 = "branch28-1"
			branch2 = "branch28-2"
		)
		branches := strings.Join([]string{branch1, branch2}, ", ")

		By("creating the RemoteSyncer first (no RemoteUser yet)")
		rs := BuildBranchRemoteSyncer(fx, "remotesyncer-test28",
			map[string]string{
				syngit.RtAnnotationKeyOneOrManyBranches: branches,
				syngit.RtAnnotationKeyUserSpecific:      string(syngit.RtAnnotationValueOneUserOneBranch),
			},
			syngit.MultipleTarget)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test28")

		By("creating the managed RemoteUser AFTER the RemoteSyncer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("the controller should back-fill RemoteTargets onto the managed RUB")
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

		By("creating a test ConfigMap")
		cm := CreateConfigMap(ctx, fx, "test-cm28", map[string]string{"test": "oui"})

		for _, br := range []string{branch1, branch2, string(utils.Developer)} {
			branch := br
			Eventually(func() (bool, error) {
				return fx.Git.IsObjectInRepo(fx.Repo, branch, cm)
			}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
		}

		ExpectOnBranch(fx, branch1, cm)
		ExpectOnBranch(fx, branch2, cm)
		ExpectOnBranch(fx, string(utils.Developer), cm)
		ExpectOnCluster(ctx, fx, "test-cm28")
	})
})
