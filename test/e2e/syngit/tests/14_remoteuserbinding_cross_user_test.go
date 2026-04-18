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
)

var _ = Describe("14 RemoteUser RBAC cross-user test", func() {

	It("prevents one user from updating another user's RemoteUser annotations", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("Developer creates their own managed RemoteUser")
		ruDev := fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, ruDev)).To(Succeed())

		By("Restricted creates their own managed RemoteUser")
		ruRes := fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", true)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted, ruRes)).To(Succeed())

		By("Developer tries to flip Restricted's managed annotation - should be denied")
		tampered := ruRes.DeepCopy()
		tampered.Annotations = map[string]string{syngit.RubAnnotationKeyManaged: "false"}
		Eventually(func() bool {
			err := fx.Users.CreateOrUpdate(ctx, utils.Developer, tampered)
			return err != nil
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("Restricted updates its own RemoteUser's annotation successfully")
		Eventually(func() error {
			return fx.Users.CreateOrUpdate(ctx, utils.Restricted, tampered)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})
})
