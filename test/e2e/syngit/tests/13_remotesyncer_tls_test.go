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

var _ = Describe("13 RemoteSyncer TLS / custom CA bundle", func() {

	It("interacts with the git server using a CA bundle secret in the same namespace (no explicit .namespace)", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the CA bundle secret")
		Expect(fx.Users.CtrlAs(utils.Developer).Create(ctx, fx.NewCABundleSecret("custom-cabundle1"))).To(Succeed())

		By("creating the managed RemoteUser pointing at TLS FQDN")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewTLSRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer with CABundleSecretRef and no explicit namespace")
		rs := BuildTLSRemoteSyncer(fx, "remotesyncer-test13-1", &corev1.SecretReference{Name: "custom-cabundle1"})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Admin, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("creating the ConfigMap - push should succeed via HTTPS")
		cm := CreateConfigMap(ctx, fx, "test-cm13-1", map[string]string{"test": "oui"})

		By("the ConfigMap is in the repo")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("the ConfigMap is on the cluster")
		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm13-1", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})

	It("interacts using a CA bundle secret in the same namespace with explicit .namespace", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the CA bundle secret")
		Expect(fx.Users.CtrlAs(utils.Developer).Create(ctx, fx.NewCABundleSecret("custom-cabundle2"))).To(Succeed())

		By("creating the managed RemoteUser pointing at TLS FQDN")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewTLSRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer with CABundleSecretRef including explicit .namespace")
		rs := BuildTLSRemoteSyncer(fx, "remotesyncer-test13-2",
			&corev1.SecretReference{Name: "custom-cabundle2", Namespace: fx.Namespace})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Admin, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		cm := CreateConfigMap(ctx, fx, "test-cm13-2", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm13-2", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})

	It("interacts using a CA bundle secret discovered in the operator namespace", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		caName := fx.TLSHost() + "-ca-cert"
		By("creating the operator-namespace CA secret with name derived from the TLS host")
		secret := fx.NewCABundleSecretInNamespace(caName, utils.OperatorNamespace)
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx, secret)).To(Succeed())
		DeferCleanup(func() {
			_ = fx.Users.CtrlAs(utils.Admin).Delete(context.Background(), secret)
		})

		By("creating the managed RemoteUser pointing at TLS FQDN")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewTLSRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer WITHOUT an explicit CABundleSecretRef - controller must discover the operator secret")
		rs := BuildTLSRemoteSyncer(fx, "remotesyncer-test13-3", nil)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Admin, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		cm := CreateConfigMap(ctx, fx, "test-cm13-3", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: "test-cm13-3", Namespace: fx.Namespace}, &corev1.ConfigMap{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})

	It("fails with an x509 error when no CA bundle is available anywhere", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the managed RemoteUser pointing at TLS FQDN")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewTLSRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer with no CA bundle reference")
		rs := BuildTLSRemoteSyncer(fx, "remotesyncer-test13-4", nil)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Admin, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		cm := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm13-4", Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err != nil && strings.Contains(err.Error(), utils.X509ErrorMessage)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})
})
