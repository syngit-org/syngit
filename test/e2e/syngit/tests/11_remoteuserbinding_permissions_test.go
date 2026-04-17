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

var _ = Describe("11 RemoteUserBinding permissions checker", func() {

	It("denies a RemoteUserBinding referencing a RemoteUser outside Restricted's RBAC", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the allowed RemoteUser for Restricted")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted,
			fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", false))).To(Succeed())

		By("Restricted tries to create a RUB referencing a RemoteUser it cannot get")
		rub := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuserbinding-restricted",
				Namespace: fx.Namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs: []corev1.ObjectReference{
					{Name: "not-allowed-remoteuser-name"},
				},
			},
		}
		Eventually(func() bool {
			err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, rub)
			return err != nil && syngitutils.ErrorTypeChecker(&syngitutils.DenyGetRemoteUserError{}, err.Error())
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})

	It("allows a RemoteUserBinding referencing a permitted RemoteUser", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the allowed RemoteUser for Restricted")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted,
			fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", false))).To(Succeed())

		By("Restricted creates the RUB referencing its own RemoteUser")
		rub := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuserbinding-restricted",
				Namespace: fx.Namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs: []corev1.ObjectReference{
					{Name: "remoteuser-restricted"},
				},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted, rub)).To(Succeed())
	})
})
