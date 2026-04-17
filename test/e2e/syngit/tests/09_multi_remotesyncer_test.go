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

	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("09 Multi RemoteSyncer test", func() {

	It("pushes a single ConfigMap to two different repos", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		repo2 := fx.SecondRepo("sunny")

		By("creating the managed RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the two RemoteSyncers (one per repo)")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildDefaultCmRemoteSyncer("remotesyncer-test9-1", fx.Namespace, "main", fx.RepoURL()))).To(Succeed())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildDefaultCmRemoteSyncer("remotesyncer-test9-2", fx.Namespace, "main", fx.RepoURLFor(repo2)))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test9-1")
		fx.WaitForDynamicWebhook("remotesyncer-test9-2")

		By("creating the ConfigMap")
		cm := CreateConfigMap(ctx, fx, "test-cm9-1", map[string]string{"test": "oui"})

		By("the ConfigMap should land in both repos")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(repo2, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("the ConfigMap is present on the cluster")
		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm9-1", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})

	It("denies a second request when two RemoteSyncers collide on the same repo", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the managed RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating two RemoteSyncers pointing at the same repo")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildDefaultCmRemoteSyncer("remotesyncer-test9-lock-1", fx.Namespace, "main", fx.RepoURL()))).To(Succeed())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildDefaultCmRemoteSyncer("remotesyncer-test9-lock-2", fx.Namespace, "main", fx.RepoURL()))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test9-lock-1")
		fx.WaitForDynamicWebhook("remotesyncer-test9-lock-2")

		By("a single push triggers both syncers; the loser reports 'incorrect old value provided'")
		cm := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm9-2", Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})

			// This error is specific to the git platform. Gitea returns "cannot lock ref"
			// while gitkit returns "incorrect old value provided".
			return err != nil && strings.Contains(err.Error(), "incorrect old value provided")
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("the ConfigMap should still be present in the repo from the successful syncer")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("the ConfigMap is NOT present on the cluster (locking error rejected the admission)")
		err := fx.Users.CtrlAs(utils.Developer).Get(ctx,
			types.NamespacedName{Name: "test-cm9-2", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		Expect(err).To(HaveOccurred())
	})
})
