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

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// newCredsSecretIn builds the basic-auth secret of a canonical user in an
// arbitrary namespace, so that a RemoteUser can point its secretRef outside of
// its own namespace and still authenticate against the git server.
func newCredsSecretIn(user utils.TestUser, name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Type:       corev1.SecretTypeBasicAuth,
		Data: map[string][]byte{
			corev1.BasicAuthUsernameKey: []byte(string(user)),
			corev1.BasicAuthPasswordKey: []byte(utils.DefaultPassword(user)),
		},
	}
}

var _ = Describe("38 Cross-namespace CRD references", func() {

	// Both sides agree on the resolved namespace, which this spec proves
	// end to end: the credentials really come from the other namespace,
	// because the push they authenticate succeeds.
	It("uses the credentials of a secretRef pointing at another namespace", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		sideNs := newSideNamespace(ctx, fx, "creds")

		By("placing Developer's credentials in " + sideNs)
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx,
			newCredsSecretIn(utils.Developer, "developer-creds-elsewhere", sideNs))).To(Succeed())

		By("creating a managed RemoteUser whose secretRef crosses namespaces")
		ru := fx.NewRemoteUser(utils.Developer, "remoteuser-test38-1", true)
		ru.Spec.SecretRef = corev1.SecretReference{
			Name: "developer-creds-elsewhere", Namespace: sideNs,
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, ru)).To(Succeed())

		By("the controller binds it to the secret of the other namespace")
		Eventually(func() syngit.SecretBoundStatus {
			fetched := &syngit.RemoteUser{}
			if err := fx.Users.CtrlAs(utils.Admin).Get(ctx,
				types.NamespacedName{Name: ru.Name, Namespace: fx.Namespace}, fetched); err != nil {
				return ""
			}
			return fetched.Status.SecretBoundStatus
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).
			Should(Equal(syngit.SecretBound))

		fx.WaitForRemoteUserReady(ru.Name)

		By("creating the RemoteSyncer and pushing through those credentials")
		rs := BuildDefaultCmRemoteSyncer("remotesyncer-test38-1", fx.Namespace, "main", fx.RepoURL())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		cm := CreateConfigMap(ctx, fx, "test-cm38-1", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})

	It("denies a RemoteUser whose secretRef crosses into an unreachable namespace", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		sideNs := newSideNamespace(ctx, fx, "ru-denied")

		By("creating a secret Restricted is not allowed to get, in " + sideNs)
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx,
			newCredsSecretIn(utils.Restricted, "unreachable-creds", sideNs))).To(Succeed())

		By("Restricted references it from its own namespace")
		ru := fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", false)
		ru.Spec.SecretRef = corev1.SecretReference{
			Name: "unreachable-creds", Namespace: sideNs,
		}

		err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, ru)
		Expect(err).To(HaveOccurred())
		Expect(syngiterrors.Is(err, syngiterrors.ErrCrossNamespaceRefDenied)).To(BeTrue(),
			"expected a cross-namespace denial, got: %v", err)
		Expect(err.Error()).To(ContainSubstring(sideNs))
		Expect(err.Error()).To(ContainSubstring("secretRef"))
	})

	// Unlike the RemoteSyncer references, a RemoteUser's own namespace is still
	// checked: being allowed to create a RemoteUser must never become a way to
	// use credentials the creator cannot read.
	It("still checks a secretRef that stays in the RemoteUser's own namespace", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating a secret Restricted is not allowed to get, in its own namespace")
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx,
			newCredsSecretIn(utils.Restricted, "local-unreachable-creds", fx.Namespace))).To(Succeed())

		ru := fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", false)
		ru.Spec.SecretRef = corev1.SecretReference{Name: "local-unreachable-creds"}

		err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, ru)
		Expect(err).To(HaveOccurred())
		Expect(syngiterrors.Is(err, syngiterrors.ErrCredentialsNotFound)).To(BeTrue(),
			"expected a same-namespace credentials denial, got: %v", err)
	})

	It("authorizes a RemoteUserBinding referencing a RemoteUser across namespaces", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		sideNs := newSideNamespace(ctx, fx, "rub-users")

		By("creating the RemoteUser Restricted is allowed to get, in " + sideNs)
		remote := fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", false)
		remote.Namespace = sideNs
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx, remote)).To(Succeed())

		By("Restricted binds to it from its own namespace")
		allowed := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuserbinding-restricted",
				Namespace: fx.Namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				Subject: rbacv1.Subject{Kind: rbacv1.UserKind, Name: string(utils.Restricted)},
				RemoteUserRefs: []corev1.ObjectReference{
					{Name: "remoteuser-restricted", Namespace: sideNs},
				},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted, allowed)).To(Succeed())

		By("but a RemoteUser it cannot get in that namespace is refused")
		denied := allowed.DeepCopy()
		denied.Spec.RemoteUserRefs = []corev1.ObjectReference{
			{Name: "not-allowed-remoteuser-name", Namespace: sideNs},
		}
		err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, denied)
		Expect(err).To(HaveOccurred())
		Expect(syngiterrors.Is(err, syngiterrors.ErrCrossNamespaceRefDenied)).To(BeTrue(),
			"expected a cross-namespace denial, got: %v", err)
	})

	It("checks the remoteTargetRefs of a RemoteUserBinding", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the RemoteUser Restricted is allowed to get")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Restricted,
			fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", false))).To(Succeed())

		By("Restricted binds a RemoteTarget, which no rule of its role allows it to get")
		rub := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuserbinding-restricted",
				Namespace: fx.Namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				Subject: rbacv1.Subject{Kind: rbacv1.UserKind, Name: string(utils.Restricted)},
				RemoteUserRefs: []corev1.ObjectReference{
					{Name: "remoteuser-restricted"},
				},
				RemoteTargetRefs: []corev1.ObjectReference{
					{Name: "any-remotetarget"},
				},
			},
		}

		err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, rub)
		Expect(err).To(HaveOccurred())
		Expect(syngiterrors.Is(err, syngiterrors.ErrRemoteTargetDenied)).To(BeTrue(),
			"expected a remote target denial, got: %v", err)
	})

	It("does not leak the namespace of one reference onto the next", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		sideNs := newSideNamespace(ctx, fx, "rub-leak")

		By("creating the RemoteUser Restricted is allowed to get, in " + sideNs)
		remote := fx.NewRemoteUser(utils.Restricted, "remoteuser-restricted", false)
		remote.Namespace = sideNs
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx, remote)).To(Succeed())

		rub := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuserbinding-restricted",
				Namespace: fx.Namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				Subject: rbacv1.Subject{Kind: rbacv1.UserKind, Name: string(utils.Restricted)},
				RemoteUserRefs: []corev1.ObjectReference{
					{Name: "remoteuser-restricted", Namespace: sideNs},
					{Name: "not-allowed-remoteuser-name"},
				},
			},
		}

		err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, rub)
		Expect(err).To(HaveOccurred())
		Expect(syngiterrors.Is(err, syngiterrors.ErrRemoteUserDenied)).To(BeTrue(),
			"the namespace-less reference must be checked in the binding's own namespace, got: %v", err)
		Expect(err.Error()).NotTo(ContainSubstring(sideNs),
			"the namespace of the first reference leaked onto the second")
	})

	// Interception must follow a cross-namespace
	// binding all the way to the credentials.
	It("intercepts through a RemoteUserBinding that references another namespace", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		sideNs := newSideNamespace(ctx, fx, "rub-runtime")

		By("placing Developer's RemoteUser and its credentials in " + sideNs)
		Expect(fx.Users.CtrlAs(utils.Admin).Create(ctx,
			newCredsSecretIn(utils.Developer, "developer-creds", sideNs))).To(Succeed())

		remoteUser := fx.NewRemoteUser(utils.Developer, "remoteuser-test38-7", false)
		remoteUser.Namespace = sideNs
		Expect(fx.Users.CtrlAs(utils.Developer).Create(ctx, remoteUser)).To(Succeed())

		By("creating the RemoteTarget in the fixture namespace")
		remoteTarget := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{Name: "remotetarget-test38-7", Namespace: fx.Namespace},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: fx.RepoURL(),
				UpstreamBranch:     "main",
				TargetRepository:   fx.RepoURL(),
				TargetBranch:       "main",
			},
		}
		Expect(fx.Users.CtrlAs(utils.Developer).Create(ctx, remoteTarget)).To(Succeed())

		By("binding Developer to the cross-namespace RemoteUser")
		rub := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuserbinding-test38-7",
				Namespace: fx.Namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				Subject: rbacv1.Subject{Kind: rbacv1.UserKind, Name: string(utils.Developer)},
				RemoteUserRefs: []corev1.ObjectReference{
					{Name: remoteUser.Name, Namespace: sideNs},
				},
				RemoteTargetRefs: []corev1.ObjectReference{
					{Name: remoteTarget.Name},
				},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rub)).To(Succeed())

		By("creating the RemoteSyncer and intercepting an object as Developer")
		rs := BuildDefaultCmRemoteSyncer("remotesyncer-test38-7", fx.Namespace, "main", fx.RepoURL())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		cm := CreateConfigMap(ctx, fx, "test-cm38-7", map[string]string{"test": "oui"})

		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cm)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})
})
