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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("01 Create RemoteUser", func() {

	It("should instantiate the RemoteUser correctly (with RemoteUserBinding)", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the RemoteUser for Developer (managed: creates a RUB)")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("checking if the RemoteUserBinding for Developer exists")
		nnRubDev := types.NamespacedName{
			Name:      fmt.Sprintf("%s-%s", syngit.RubNamePrefix, utils.SanitizeUser(utils.Developer)),
			Namespace: fx.Namespace,
		}
		rubDev := &syngit.RemoteUserBinding{}
		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx, nnRubDev, rubDev)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("creating the RemoteUser for Restricted (unmanaged: no RUB)")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted,
			fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", false))).To(Succeed())

		By("checking that the RemoteUserBinding for Restricted does not exist")
		nnRubRes := types.NamespacedName{
			Name:      fmt.Sprintf("%s-%s", syngit.RubNamePrefix, utils.SanitizeUser(utils.Restricted)),
			Namespace: fx.Namespace,
		}
		rubRes := &syngit.RemoteUserBinding{}
		err := fx.Users.CtrlAs(utils.Restricted).Get(ctx, nnRubRes, rubRes)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(utils.NotFoundMessage))
	})
})
