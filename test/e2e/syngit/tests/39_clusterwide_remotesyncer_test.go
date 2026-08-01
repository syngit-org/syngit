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

	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/pkg/interceptor"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// managedLabel marks the namespaces a ClusterWideRemoteSyncer's
// namespaceSelector picks up.
var managedLabel = map[string]string{"syngit.io/test39": "true"}

// createConfigMapIn creates a ConfigMap in an arbitrary namespace as Developer.
//
// It goes through the typed clientset, like the other specs do, and returns the
// locally built object: the controller-runtime client would overwrite it with
// the server's response, which carries no TypeMeta, and the repo assertions
// match on apiVersion and kind.
func createConfigMapIn(ctx context.Context, fx *utils.Fixture, ns, name string) *corev1.ConfigMap {
	GinkgoHelper()
	cm := &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       map[string]string{"test": "oui"},
	}
	Eventually(func() error {
		_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(ns).
			Create(ctx, cm, metav1.CreateOptions{})
		return err
	}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	return cm
}

var _ = Describe("39 Cluster-wide remote syncer", func() {

	It("intercepts several namespaces at once, filing each object under its own namespace", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		nsA := fx.NewLabeledNamespace("a", managedLabel)
		nsB := fx.NewLabeledNamespace("b", managedLabel)

		By("creating the RemoteUser & managed RemoteUserBinding in the identity namespace")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())
		fx.WaitForRemoteUserReady("remoteuser-developer")

		By("creating the ClusterWideRemoteSyncer over both namespaces")
		cwrs := BuildClusterWideRemoteSyncer(fx, "cwrsy-test39-1",
			&metav1.LabelSelector{MatchLabels: managedLabel},
			[]admissionv1.RuleWithOperations{ConfigMapRule()})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Admin, cwrs)).To(Succeed())
		DeferCleanup(func() {
			_ = fx.Users.CtrlAs(utils.Admin).Delete(context.Background(), cwrs)
		})
		fx.WaitForClusterWideDynamicWebhook(cwrs.Name)

		By("creating a ConfigMap in each of the two namespaces")
		cmA := createConfigMapIn(ctx, fx, nsA, "test-cm39-a")
		cmB := createConfigMapIn(ctx, fx, nsB, "test-cm39-b")

		By("both reached the single repository")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cmA)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cmB)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("each is filed under the namespace it came from, not the syncer's")
		filesA, err := fx.Git.SearchForObjectInRepo(fx.Repo, "main", cmA)
		Expect(err).NotTo(HaveOccurred())
		Expect(filesA).NotTo(BeEmpty())
		Expect(filesA[0].Path).To(HavePrefix(nsA+"/"),
			"a cluster-wide syncer must file an object under the namespace it was intercepted in")

		filesB, err := fx.Git.SearchForObjectInRepo(fx.Repo, "main", cmB)
		Expect(err).NotTo(HaveOccurred())
		Expect(filesB).NotTo(BeEmpty())
		Expect(filesB[0].Path).To(HavePrefix(nsB + "/"))
	})

	It("files a cluster-scoped resource under the cluster-scoped path segment", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the RemoteUser & managed RemoteUserBinding in the identity namespace")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())
		fx.WaitForRemoteUserReady("remoteuser-developer")

		By("creating the ClusterWideRemoteSyncer scoping a cluster-scoped kind")
		cwrs := BuildClusterWideRemoteSyncer(fx, "cwrsy-test39-2", nil,
			[]admissionv1.RuleWithOperations{ClusterRoleRule()})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Admin, cwrs)).To(Succeed())
		DeferCleanup(func() {
			_ = fx.Users.CtrlAs(utils.Admin).Delete(context.Background(), cwrs)
		})
		fx.WaitForClusterWideDynamicWebhook(cwrs.Name)

		By("creating a ClusterRole, which belongs to no namespace")
		cr := &rbacv1.ClusterRole{
			TypeMeta: metav1.TypeMeta{Kind: "ClusterRole", APIVersion: "rbac.authorization.k8s.io/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: fx.Namespace + "-test39-clusterrole",
			},
			Rules: []rbacv1.PolicyRule{{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get"},
			}},
		}
		Eventually(func() error {
			_, err := fx.Users.KAs(utils.Developer).RbacV1().ClusterRoles().
				Create(ctx, cr, metav1.CreateOptions{})
			return err
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
		DeferCleanup(func() {
			_ = fx.Users.CtrlAs(utils.Admin).Delete(context.Background(), cr)
		})

		By("it reached the repository")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cr)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("and is filed under the cluster-scoped segment")
		// The path is read off the file listing rather than from
		// SearchForObjectInRepo: that helper additionally requires the document
		// to carry a spec or data field, which a ClusterRole (whose payload is
		// "rules") never does.
		files, err := fx.Git.ListFiles(fx.Repo, "main")
		Expect(err).NotTo(HaveOccurred())

		var path string
		for _, f := range files {
			if strings.HasSuffix(f, "/"+cr.Name+".yaml") {
				path = f
				break
			}
		}
		Expect(path).NotTo(BeEmpty(), "no file named after the ClusterRole; files=%v", files)
		Expect(path).To(HavePrefix(interceptor.ClusterScopedPathSegment+"/"),
			"a cluster-scoped object has no namespace, so it must be filed under %q; got %q",
			interceptor.ClusterScopedPathSegment, path)
	})

	It("updates a cluster-scoped resource in place at its pre-existing path", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the RemoteUser & managed RemoteUserBinding in the identity namespace")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())
		fx.WaitForRemoteUserReady("remoteuser-developer")

		cwrs := BuildClusterWideRemoteSyncer(fx, "cwrsy-test39-3", nil,
			[]admissionv1.RuleWithOperations{ClusterRoleRule()})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Admin, cwrs)).To(Succeed())
		DeferCleanup(func() {
			_ = fx.Users.CtrlAs(utils.Admin).Delete(context.Background(), cwrs)
		})
		fx.WaitForClusterWideDynamicWebhook(cwrs.Name)

		cr := &rbacv1.ClusterRole{
			TypeMeta: metav1.TypeMeta{Kind: "ClusterRole", APIVersion: "rbac.authorization.k8s.io/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: fx.Namespace + "-test39-finder-clusterrole",
			},
			Rules: []rbacv1.PolicyRule{{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get"},
			}},
		}

		By("pre-committing the ClusterRole at a custom path in the repo")
		const customPath = "custom-path/finder-clusterrole.yaml"
		Expect(fx.Git.CommitObject(fx.Repo, "main", customPath, cr,
			"seed finder-clusterrole.yaml")).To(Succeed())

		By("creating it on the cluster with an extra resource")
		// A cluster-scoped object carries no namespace, so the finder's selector
		// has an empty one. It must still match the seeded document, whose
		// metadata carries no namespace either.
		cr.Rules[0].Resources = append(cr.Rules[0].Resources, "services")
		Eventually(func() error {
			_, err := fx.Users.KAs(utils.Developer).RbacV1().ClusterRoles().
				Create(ctx, cr, metav1.CreateOptions{})
			return err
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
		DeferCleanup(func() {
			_ = fx.Users.CtrlAs(utils.Admin).Delete(context.Background(), cr)
		})

		By("the seeded file is the one that carries the update")
		Eventually(func() (string, error) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", customPath)
			return string(content), err
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).
			Should(ContainSubstring("services"))

		By("and nothing was written under the cluster-scoped segment")
		files, err := fx.Git.ListFiles(fx.Repo, "main")
		Expect(err).NotTo(HaveOccurred())
		var duplicates []string
		for _, f := range files {
			if strings.HasPrefix(f, interceptor.ClusterScopedPathSegment+"/") &&
				strings.HasSuffix(f, "/"+cr.Name+".yaml") {
				duplicates = append(duplicates, f)
			}
		}
		Expect(duplicates).To(BeEmpty(),
			"finding the existing document must win over the default layout, "+
				"otherwise the ClusterRole ends up recorded twice")
	})

	It("leaves a namespace that does not match the selector alone", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		selected := fx.NewLabeledNamespace("selected", managedLabel)
		ignored := fx.NewLabeledNamespace("ignored", map[string]string{"syngit.io/test39": "false"})

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())
		fx.WaitForRemoteUserReady("remoteuser-developer")

		cwrs := BuildClusterWideRemoteSyncer(fx, "cwrsy-test39-4",
			&metav1.LabelSelector{MatchLabels: managedLabel},
			[]admissionv1.RuleWithOperations{ConfigMapRule()})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Admin, cwrs)).To(Succeed())
		DeferCleanup(func() {
			_ = fx.Users.CtrlAs(utils.Admin).Delete(context.Background(), cwrs)
		})
		fx.WaitForClusterWideDynamicWebhook(cwrs.Name)

		By("a ConfigMap in the selected namespace is pushed")
		cmSelected := createConfigMapIn(ctx, fx, selected, "test-cm39-selected")
		Eventually(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cmSelected)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("a ConfigMap in the unselected namespace is never pushed")
		cmIgnored := createConfigMapIn(ctx, fx, ignored, "test-cm39-ignored")
		Consistently(func() (bool, error) {
			return fx.Git.IsObjectInRepo(fx.Repo, "main", cmIgnored)
		}).WithTimeout(3*utils.DefaultInterval).Should(BeFalse(),
			"the namespaceSelector must keep unmatched namespaces out of the syncer")

		By("but it still exists on the cluster, untouched")
		Expect(fx.Users.CtrlAs(utils.Admin).Get(ctx,
			types.NamespacedName{Name: "test-cm39-ignored", Namespace: ignored},
			&corev1.ConfigMap{})).To(Succeed())
	})

	It("rejects a reference that carries no namespace", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		cwrs := BuildClusterWideRemoteSyncer(fx, "cwrsy-test39-5", nil,
			[]admissionv1.RuleWithOperations{ConfigMapRule()})
		// A cluster-scoped object has no namespace of its own, so this reference
		// cannot be resolved.
		cwrs.Spec.CABundleSecretRef = corev1.SecretReference{Name: "some-cabundle"}

		err := fx.Users.CreateOrUpdate(ctx, utils.Admin, cwrs)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("caBundleSecretRef"))
		Expect(strings.ToLower(err.Error())).To(ContainSubstring("namespace"))
	})

	It("denies a user who cannot reach the identity namespace", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("Restricted asks for a syncer drawing identities from a namespace it cannot list")
		cwrs := BuildClusterWideRemoteSyncer(fx, "cwrsy-test39-6", nil,
			[]admissionv1.RuleWithOperations{{
				Operations: []admissionv1.OperationType{admissionv1.Create},
				Rule: admissionv1.Rule{
					APIGroups:   []string{""},
					APIVersions: []string{"v1"},
					Resources:   []string{"secrets"},
				},
			}})
		cwrs.Spec.IdentityStoreNamespace = utils.OperatorNamespace

		err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, cwrs)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("remoteuserbindings"),
			"pointing a syncer at an identity namespace must require listing its RemoteUserBindings")
	})

	It("denies a user who cannot act on the scoped resources cluster-wide", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("Restricted asks to intercept a kind it has no cluster-wide rights on")
		cwrs := BuildClusterWideRemoteSyncer(fx, "cwrsy-test39-7", nil,
			[]admissionv1.RuleWithOperations{ClusterRoleRule()})

		err := fx.Users.CreateOrUpdate(ctx, utils.Restricted, cwrs)
		Expect(err).To(HaveOccurred())
		Expect(syngiterrors.Is(err, syngiterrors.ErrResourceScopeForbidden)).To(BeTrue(),
			"expected a resource-scope denial, got: %v", err)
	})

	It("removes its webhook entry when deleted, leaving other syncers alone", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())
		fx.WaitForRemoteUserReady("remoteuser-developer")

		By("creating a namespaced RemoteSyncer and a cluster-wide one side by side")
		rs := BuildDefaultCmRemoteSyncer("remotesyncer-test39-8", fx.Namespace, "main", fx.RepoURL())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		cwrs := BuildClusterWideRemoteSyncer(fx, "cwrsy-test39-9",
			&metav1.LabelSelector{MatchLabels: managedLabel},
			[]admissionv1.RuleWithOperations{ConfigMapRule()})
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Admin, cwrs)).To(Succeed())
		fx.WaitForClusterWideDynamicWebhook(cwrs.Name)

		By("deleting the cluster-wide one")
		Expect(fx.Users.CtrlAs(utils.Admin).Delete(ctx, cwrs)).To(Succeed())

		By("its entry goes away")
		Eventually(func() bool {
			return !hasWebhookEntry(fx, cwrs.Name+".clusterwide.syngit.io")
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		By("and the namespaced syncer's entry is still there")
		Consistently(func() bool {
			return hasWebhookEntry(fx, rs.Name+"."+fx.Namespace+".syngit.io")
		}).WithTimeout(3*utils.DefaultInterval).Should(BeTrue(),
			"removing one syncer's entry must not disturb another's")
	})
})

// hasWebhookEntry reports whether the shared dynamic configuration carries an
// entry of the given name.
func hasWebhookEntry(fx *utils.Fixture, name string) bool {
	vwc := &admissionv1.ValidatingWebhookConfiguration{}
	if err := fx.Users.CtrlAs(utils.Admin).Get(fx.Ctx,
		types.NamespacedName{Name: utils.DynamicWebhookName}, vwc); err != nil {
		return false
	}
	for _, w := range vwc.Webhooks {
		if w.Name == name {
			return true
		}
	}
	return false
}
