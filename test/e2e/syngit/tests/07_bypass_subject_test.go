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
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("07 Subject bypasses interception", func() {

	bypassSpec := func(strategy syngit.Strategy, rsName, cmName string) {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the managed RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating a RemoteSyncer bypassing the Developer subject")
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rsName,
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: "main",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				BypassInterceptionSubjects: []rbacv1.Subject{{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "User",
					Name:     string(utils.Developer),
				}},
				ExcludedFields:   []string{".metadata.uid"},
				Strategy:         strategy,
				TargetStrategy:   syngit.OneTarget,
				RemoteRepository: fx.RepoURL(),
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{admissionv1.Create},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"configmaps"},
						},
					}},
				},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("creating a test ConfigMap (should bypass interception)")
		cm := CreateConfigMap(ctx, fx, cmName, map[string]string{"test": "oui"})

		By("the ConfigMap should NOT be pushed to the git repo")
		Consistently(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(3 * utils.DefaultInterval).Should(BeFalse())

		By("the ConfigMap should be present on the cluster")
		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: cmName, Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	}

	It("applies the resource on the cluster but not in git (CommitApply)", func() {
		bypassSpec(syngit.CommitApply, "remotesyncer-test7-1", "test-cm7-1")
	})

	It("applies the resource on the cluster but not in git (CommitOnly)", func() {
		bypassSpec(syngit.CommitOnly, "remotesyncer-test7-2", "test-cm7-2")
	})
})
