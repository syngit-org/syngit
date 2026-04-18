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
	syngitutils "github.com/syngit-org/syngit/pkg/utils"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("10 RemoteUser secret permissions checker", func() {

	It("denies RemoteUser creation when the referenced secret is outside Restricted's RBAC", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		ru := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuser-restricted",
				Namespace: fx.Namespace,
			},
			Spec: syngit.RemoteUserSpec{
				Email:             utils.DefaultEmail(utils.Restricted),
				GitBaseDomainFQDN: fx.FQDN(),
				SecretRef: corev1.SecretReference{
					Name: "not-allowed-secret-name",
				},
			},
		}
		Eventually(func() bool {
			err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, ru)
			return err != nil && syngitutils.ErrorTypeChecker(&syngitutils.DenyGetSecretError{}, err.Error())
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})

	It("allows RemoteUser creation when the referenced secret is within Restricted's RBAC", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted,
			fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", false))).To(Succeed())
	})
})
