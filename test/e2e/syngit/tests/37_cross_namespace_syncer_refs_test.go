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

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// newSideNamespace creates a namespace next to the fixture's own one and
// registers its deletion. Cross-namespace references point into it.
func newSideNamespace(ctx context.Context, fx *utils.Fixture, suffix string) string {
	GinkgoHelper()
	name := fx.Namespace + "-" + suffix
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx, ns)).To(Succeed())
	DeferCleanup(func() {
		_ = fx.Users.CtrlAs(utils.Admin).Delete(context.Background(), ns)
	})
	return name
}

// buildSecretScopedRS builds a RemoteSyncer scoping "create secrets", which is
// the only interception scope the Restricted user is allowed to ask for. Using
// it means the rules-permissions check passes, so a denial can only come from
// the cross-namespace reference check.
func buildSecretScopedRS(fx *utils.Fixture, name string, caRef corev1.SecretReference) *syngit.RemoteSyncer {
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
			ExcludedFields:              []string{".metadata.uid"},
			Strategy:                    syngit.CommitApply,
			TargetStrategy:              syngit.OneTarget,
			RemoteRepository:            fx.RepoURL(),
			CABundleSecretRef:           caRef,
			ScopedResources: syngit.ScopedResources{
				Rules: []admissionv1.RuleWithOperations{{
					Operations: []admissionv1.OperationType{admissionv1.Create},
					Rule: admissionv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"secrets"},
					},
				}},
			},
		},
	}
}

var _ = Describe("37 Cross-namespace references", func() {

	It("reads a CA bundle from another namespace when the writer is allowed to get it", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		sideNs := newSideNamespace(ctx, fx, "ca")

		By("creating the CA bundle secret in " + sideNs)
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx,
			fx.NewCABundleSecretInNamespace("custom-cabundle37", sideNs))).To(Succeed())

		By("creating the managed RemoteUser pointing at the TLS FQDN")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewTLSRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("Developer creates the RemoteSyncer referencing the CA bundle across namespaces")
		rs := BuildTLSRemoteSyncer(fx, "remotesyncer-test37-1",
			&corev1.SecretReference{Name: "custom-cabundle37", Namespace: sideNs})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("the push over HTTPS succeeds, so the CA bundle was really read from " + sideNs)
		cm := CreateConfigMap(ctx, fx, "test-cm37-1", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})

	It("denies at admission a cross-namespace reference the writer cannot get", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		sideNs := newSideNamespace(ctx, fx, "denied")

		By("creating the CA bundle secret in " + sideNs)
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx,
			fx.NewCABundleSecretInNamespace("unreachable-cabundle", sideNs))).To(Succeed())

		By("Restricted tries to reference it from its own namespace")
		rs := buildSecretScopedRS(fx, "remotesyncer-test37-2",
			corev1.SecretReference{Name: "unreachable-cabundle", Namespace: sideNs})

		err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, rs)
		Expect(err).To(HaveOccurred())
		Expect(syngiterrors.Is(err, syngiterrors.ErrCrossNamespaceRefDenied)).To(BeTrue(),
			"expected a cross-namespace denial, got: %v", err)
		Expect(err.Error()).To(ContainSubstring(sideNs))
		Expect(err.Error()).To(ContainSubstring("caBundleSecretRef"))
	})

	It("never checks a same-namespace reference, even one the writer cannot get", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating a secret Restricted is not allowed to get, in its own namespace")
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx,
			fx.NewCABundleSecretInNamespace("local-cabundle37", fx.Namespace))).To(Succeed())

		By("Restricted references it without a namespace: the reference is not authorized at all")
		rsImplicit := buildSecretScopedRS(fx, "remotesyncer-test37-3a",
			corev1.SecretReference{Name: "local-cabundle37"})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted, rsImplicit)).To(Succeed())

		By("spelling out its own namespace resolves to the same thing and stays unchecked")
		rsExplicit := buildSecretScopedRS(fx, "remotesyncer-test37-3b",
			corev1.SecretReference{Name: "local-cabundle37", Namespace: fx.Namespace})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted, rsExplicit)).To(Succeed())
	})

	It("denies an update that repoints a reference at an unreachable namespace", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		sideNs := newSideNamespace(ctx, fx, "update")

		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx,
			fx.NewCABundleSecretInNamespace("local-cabundle37", fx.Namespace))).To(Succeed())
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx,
			fx.NewCABundleSecretInNamespace("unreachable-cabundle", sideNs))).To(Succeed())

		By("Restricted creates a RemoteSyncer referencing its own namespace")
		rs := buildSecretScopedRS(fx, "remotesyncer-test37-4",
			corev1.SecretReference{Name: "local-cabundle37"})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted, rs)).To(Succeed())

		By("Restricted repoints the reference at a namespace it cannot read")
		rs.Spec.CABundleSecretRef = corev1.SecretReference{
			Name: "unreachable-cabundle", Namespace: sideNs,
		}
		err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, rs)
		Expect(err).To(HaveOccurred())
		Expect(syngiterrors.Is(err, syngiterrors.ErrCrossNamespaceRefDenied)).To(BeTrue(),
			"expected a cross-namespace denial on update, got: %v", err)
	})

	It("denies the intercepted request when its author cannot get a cross-namespace reference", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		sideNs := newSideNamespace(ctx, fx, "runtime")

		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx,
			fx.NewCABundleSecretInNamespace("unreachable-cabundle", sideNs))).To(Succeed())

		By("Developer, who may read every namespace, creates the RemoteSyncer")
		rs := buildSecretScopedRS(fx, "remotesyncer-test37-5",
			corev1.SecretReference{Name: "unreachable-cabundle", Namespace: sideNs})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("Restricted creates an intercepted secret and is denied on the reference")
		sec := &corev1.Secret{
			TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-secret37-5", Namespace: fx.Namespace},
			StringData: map[string]string{"test": "test1"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Restricted).CoreV1().Secrets(fx.Namespace).
				Create(ctx, sec, metav1.CreateOptions{})
			return err != nil && syngiterrors.Is(err, syngiterrors.ErrCrossNamespaceRefDenied)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue(),
			fmt.Sprintf("expected the intercepted create to be denied on the reference into %s", sideNs))
	})

	It("exempts the manager namespace from the runtime check", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating a CA bundle secret in the manager namespace that Restricted cannot get")
		shared := fx.NewCABundleSecretInNamespace("shared-cabundle37", utils.OperatorNamespace)
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx, shared)).To(Succeed())
		DeferCleanup(func() {
			_ = fx.Users.CtrlAs(utils.Admin).Delete(context.Background(), shared)
		})

		By("creating the managed RemoteUser for Restricted")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted,
			fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", true))).To(Succeed())
		fx.WaitForRemoteUserReady("remoteuser-restricted")

		By("Developer creates the RemoteSyncer referencing the manager namespace")
		rs := buildSecretScopedRS(fx, "remotesyncer-test37-6",
			corev1.SecretReference{Name: "shared-cabundle37", Namespace: utils.OperatorNamespace})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("Restricted's intercepted create goes through despite not being able to get that secret")
		sec := &corev1.Secret{
			TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-secret37-6", Namespace: fx.Namespace},
			StringData: map[string]string{"test": "test1"},
		}
		Eventually(func() error {
			_, err := fx.Users.KAs(utils.Restricted).CoreV1().Secrets(fx.Namespace).
				Create(ctx, sec, metav1.CreateOptions{})
			return err
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("the secret reached the repo and the cluster")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", sec)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		Eventually(func() error {
			return fx.Users.CtrlAs(utils.Admin).Get(ctx,
				types.NamespacedName{Name: "test-secret37-6", Namespace: fx.Namespace}, &corev1.Secret{})
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})
})
