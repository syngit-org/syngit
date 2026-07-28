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

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("34 All ServiceAccounts bypass interception", func() {

	// buildRS returns a CommitApply RemoteSyncer on ConfigMaps whose
	// bypassAllServiceAccounts is set to bypass. Every subject that is not
	// bypassed is blocked when it has no RemoteUserBinding.
	buildRS := func(fx *utils.Fixture, name string, bypass bool) *syngit.RemoteSyncer {
		return &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: "main",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.BlockDefaultUser,
				BypassAllServiceAccounts:    bypass,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            fx.RepoURL(),
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
	}

	// createCMAs creates a ConfigMap impersonating user and returns the
	// object along with the error returned by the apiserver, so specs can
	// assert on interception denials.
	createCMAs := func(ctx context.Context, fx *utils.Fixture,
		user utils.TestUser, name string) (*corev1.ConfigMap, error) {
		cm := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
		_, err := fx.Users.KAs(user).CoreV1().ConfigMaps(fx.Namespace).
			Create(ctx, cm, metav1.CreateOptions{})
		return cm, err
	}

	It("applies the ServiceAccount's resource on the cluster but not in git", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the ServiceAccount that will apply the ConfigMap")
		sa := fx.NewServiceAccount(ctx, "bypassed-sa")

		By("creating the managed RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating a RemoteSyncer bypassing every ServiceAccount")
		rs := buildRS(fx, "remotesyncer-test34-1", true)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("creating a ConfigMap as the ServiceAccount (should bypass interception)")
		var cm *corev1.ConfigMap
		Eventually(func() error {
			var err error
			cm, err = createCMAs(ctx, fx, sa, "test-cm34-1")
			return err
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("the ConfigMap should NOT be pushed to the git repo")
		Consistently(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(3 * utils.DefaultInterval).Should(BeFalse())

		By("the ConfigMap should be present on the cluster")
		ExpectOnCluster(ctx, fx, "test-cm34-1")
	})

	It("still intercepts regular users when ServiceAccounts are bypassed", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the managed RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating a RemoteSyncer bypassing every ServiceAccount")
		rs := buildRS(fx, "remotesyncer-test34-2", true)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("creating a ConfigMap as Developer, who is not a ServiceAccount")
		cm := CreateConfigMap(ctx, fx, "test-cm34-2", map[string]string{"test": "oui"})

		By("the ConfigMap should be pushed to the git repo")
		ExpectOnBranch(fx, "main", cm)

		By("the ConfigMap should be present on the cluster")
		ExpectOnCluster(ctx, fx, "test-cm34-2")
	})

	It("intercepts the ServiceAccount when the bypass is disabled", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the ServiceAccount that will apply the ConfigMap")
		sa := fx.NewServiceAccount(ctx, "intercepted-sa")

		By("creating the managed RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating a RemoteSyncer that does not bypass ServiceAccounts")
		rs := buildRS(fx, "remotesyncer-test34-3", false)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("the ServiceAccount has no RemoteUserBinding, so the create is denied")
		var cm *corev1.ConfigMap
		Eventually(func() bool {
			var err error
			cm, err = createCMAs(ctx, fx, sa, "test-cm34-3")
			return err != nil && syngiterrors.Is(err, syngiterrors.ErrRemoteUserBindingNotFound)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("the ConfigMap is neither on the cluster nor in the git repo")
		Eventually(func() bool {
			err := fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm34-3", Namespace: fx.Namespace}, &corev1.ConfigMap{})
			return err != nil && strings.Contains(err.Error(), utils.NotFoundMessage)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
		ExpectNotOnBranch(fx, "main", cm)
	})
})
