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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("06 Test objects lifecycle", func() {

	It("properly manages the RemoteUserBinding across two distinct git hosts", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the RemoteUser for Developer on the primary git server")
		ruPrimary := fx.NewRemoteUser(utils.Developer, "remoteuser-developer-primary", true)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, ruPrimary)).To(Succeed())

		By("creating the RemoteUser for Developer on the alternate git server")
		ruAlt := fx.NewRemoteUser(utils.Developer, "remoteuser-developer-alt", true)
		ruAlt.Spec.GitBaseDomainFQDN = fx.AltFQDN()
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, ruAlt)).To(Succeed())

		By("updating the alternate RemoteUser to exercise an update path")
		ruAlt.Annotations["change"] = "something"
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, ruAlt)).To(Succeed())

		nnRub := types.NamespacedName{
			Name:      fmt.Sprintf("%s-%s", syngit.RubNamePrefix, utils.SanitizeUser(utils.Developer)),
			Namespace: fx.Namespace,
		}
		By("RemoteUserBinding should aggregate both RemoteUsers")
		Eventually(func() (int, error) {
			got := &syngit.RemoteUserBinding{}
			if err := fx.Users.CtrlAs(utils.Developer).Get(ctx, nnRub, got); err != nil {
				return 0, err
			}
			return len(got.Spec.RemoteUserRefs), nil
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Equal(2))

		By("deleting the alt RemoteUser")
		Expect(fx.Users.CtrlAs(utils.Developer).Delete(ctx, ruAlt)).To(Succeed())

		By("RemoteUserBinding should now reference only the primary RemoteUser")
		Eventually(func() (int, error) {
			got := &syngit.RemoteUserBinding{}
			if err := fx.Users.CtrlAs(utils.Developer).Get(ctx, nnRub, got); err != nil {
				return 0, err
			}
			return len(got.Spec.RemoteUserRefs), nil
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Equal(1))

		By("deleting the primary RemoteUser")
		Expect(fx.Users.CtrlAs(utils.Developer).Delete(ctx, ruPrimary)).To(Succeed())

		By("RemoteUserBinding should disappear")
		Eventually(func() bool {
			err := fx.Users.CtrlAs(utils.Developer).Get(ctx, nnRub, &syngit.RemoteUserBinding{})
			return err != nil && strings.Contains(err.Error(), utils.NotFoundMessage)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})

	It("properly manages the RemoteSyncer associated dynamic webhooks", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating a RemoteSyncer scoped to StatefulSets")
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test6",
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: "main",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            fx.RepoURL(),
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionregistrationv1.RuleWithOperations{{
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"statefulsets"},
						},
					}},
				},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())

		By("expecting the dynamic validating webhook to include exactly one statefulsets rule")
		Eventually(func() (int, error) {
			got := &admissionregistrationv1.ValidatingWebhookConfiguration{}
			err := fx.Users.CtrlAs(utils.Admin).Get(ctx,
				types.NamespacedName{Name: utils.DynamicWebhookName}, got)
			if err != nil {
				return 0, err
			}
			count := 0
			for _, w := range got.Webhooks {
				if len(w.Rules) > 0 && len(w.Rules[0].Resources) > 0 &&
					w.Rules[0].Resources[0] == "statefulsets" {
					count++
				}
			}
			return count, nil
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Equal(1))
	})
})
