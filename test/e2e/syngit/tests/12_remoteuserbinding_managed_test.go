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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("12 RemoteUserBinding managed-by checker", func() {

	It("generates a numeric suffix when the canonical RUB name is taken", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating an unmanaged RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", false))).To(Succeed())

		canonicalName := fmt.Sprintf("%s-%s", syngit.RubNamePrefix, utils.SanitizeUser(utils.Developer))
		By("creating a RUB manually at the canonical name (collides with the managed one)")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      canonicalName,
				Namespace: fx.Namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs: []corev1.ObjectReference{{Name: "remoteuser-developer"}},
			},
		})).To(Succeed())

		By("creating a second RUB at suffix -1 (also collides)")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      canonicalName + "-1",
				Namespace: fx.Namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs: []corev1.ObjectReference{{Name: "remoteuser-developer"}},
			},
		})).To(Succeed())

		By("updating the RemoteUser to use managed association")
		managedRU := fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, managedRU)).To(Succeed())

		By("the controller should create a RUB at suffix -2")
		suffixedName := canonicalName + "-2"
		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: suffixedName, Namespace: fx.Namespace},
				&syngit.RemoteUserBinding{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})
})
