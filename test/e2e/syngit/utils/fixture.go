package utils

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2" // nolint:staticcheck
	. "github.com/onsi/gomega"    // nolint:staticcheck
	"sigs.k8s.io/controller-runtime/pkg/client"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	syngitenvtest "github.com/syngit-org/syngit/pkg/envtest"
)

// seq produces monotonically increasing suffixes so each spec gets
// globally unique repo and namespace names within one suite run.
var seq atomic.Uint64

// Fixture holds per-spec state: a fresh Kubernetes namespace, a fresh
// repository on the primary git server with baseline permissions, and
// basic-auth secrets for each canonical user. It also exposes the two
// shared GitServers for tests that need multi-server behavior.
//
// Construct a Fixture once per It block via Suite.NewFixture(ctx).
type Fixture struct {
	Ctx       context.Context
	Namespace string
	Repo      syngitenvtest.RepoRef

	Git    *syngitenvtest.GitServer
	GitAlt *syngitenvtest.GitServer
	Users  *UserClient
}

// NewFixture is the Suite-aware factory for Fixture. It creates a
// uniquely-named namespace, a uniquely-named repo on the primary server
// with the baseline permissions granted (admin+developer ReadWrite,
// restricted NoAccess), basic-auth secrets for every canonical user in
// the namespace, and registers DeferCleanup to delete the namespace at
// spec end.
func (s *Suite) NewFixture(ctx context.Context) *Fixture {
	GinkgoHelper()
	id := seq.Add(1)

	f := &Fixture{
		Ctx:       ctx,
		Namespace: fmt.Sprintf("e2e-%d", id),
		Repo:      syngitenvtest.RepoRef{Owner: RepoOwner, Name: fmt.Sprintf("repo-%d", id)},
		Git:       s.GitServer,
		GitAlt:    s.GitServerAlt,
		Users:     s.Users,
	}

	By("creating namespace " + f.Namespace)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: f.Namespace}}
	Expect(f.Users.CtrlAs(Admin).Create(ctx, ns)).To(Succeed())
	DeferCleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
		defer cancel()
		cl := f.Users.CtrlAs(Admin)

		// Remove finalizers so the namespace is not stuck in Terminating
		// (envtest has no namespace controller to process them).
		got := &corev1.Namespace{}
		if err := cl.Get(cleanupCtx, types.NamespacedName{Name: f.Namespace}, got); err == nil {
			if len(got.Spec.Finalizers) > 0 || len(got.Finalizers) > 0 {
				got.Spec.Finalizers = nil
				got.Finalizers = nil
				_ = cl.Update(cleanupCtx, got)
			}
		}

		// Force-delete with zero grace period.
		zero := int64(0)
		_ = cl.Delete(cleanupCtx, ns, &client.DeleteOptions{
			GracePeriodSeconds: &zero,
		})
	})

	By("creating repo " + f.Repo.String())
	Expect(f.Git.CreateRepo(f.Repo, "main")).To(Succeed())
	grantBaseline(f.Git, f.Repo)

	By("creating basic-auth secrets for each user in the namespace")
	for _, u := range AllUsers {
		sec := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      string(u) + "-creds",
				Namespace: f.Namespace,
			},
			Type: corev1.SecretTypeBasicAuth,
			Data: map[string][]byte{
				corev1.BasicAuthUsernameKey: []byte(string(u)),
				corev1.BasicAuthPasswordKey: []byte(DefaultPassword(u)),
			},
		}
		if err := f.Users.CtrlAs(Admin).Create(ctx, sec); err != nil && !apierrors.IsAlreadyExists(err) {
			Fail(fmt.Sprintf("create %s secret: %v", u, err))
		}
	}

	return f
}

// grantBaseline applies the standard permission matrix on a repo: all
// three canonical users receive ReadWrite. Tests that need to simulate
// a "no git access" scenario should point a RemoteUser at the secret
// returned by NewBogusCredsSecret rather than revoking permissions on
// a canonical user.
func grantBaseline(gs *syngitenvtest.GitServer, repo syngitenvtest.RepoRef) {
	gs.SetPermission(string(Admin), repo, syngitenvtest.ReadWrite)
	gs.SetPermission(string(Developer), repo, syngitenvtest.ReadWrite)
	gs.SetPermission(string(Restricted), repo, syngitenvtest.ReadWrite)
}

// NewBogusCredsSecret builds a kubernetes.io/basic-auth secret with
// credentials that are not registered on either git server, so git auth
// will always fail when a RemoteUser points at this secret. Used for
// "no git access" scenarios.
func (f *Fixture) NewBogusCredsSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: f.Namespace},
		Type:       corev1.SecretTypeBasicAuth,
		Data: map[string][]byte{
			corev1.BasicAuthUsernameKey: []byte("unknown-user"),
			corev1.BasicAuthPasswordKey: []byte("wrong-password"),
		},
	}
}

// --- URL / FQDN helpers ----------------------------------------------------

// RepoURL returns the plain-HTTP clone URL for the primary repo.
func (f *Fixture) RepoURL() string { return f.Git.RepoURL(f.Repo) }

// RepoURLFor returns the plain-HTTP clone URL for any repo on the primary server.
func (f *Fixture) RepoURLFor(repo syngitenvtest.RepoRef) string { return f.Git.RepoURL(repo) }

// TLSRepoURL returns the HTTPS clone URL for the primary repo.
func (f *Fixture) TLSRepoURL() string { return f.Git.TLSRepoURL(f.Repo) }

// TLSRepoURLFor returns the HTTPS clone URL for any repo on the primary server.
func (f *Fixture) TLSRepoURLFor(repo syngitenvtest.RepoRef) string { return f.Git.TLSRepoURL(repo) }

// FQDN returns host:port of the primary HTTP listener.
func (f *Fixture) FQDN() string { return f.Git.FQDN() }

// TLSFQDN returns host:port of the primary HTTPS listener.
func (f *Fixture) TLSFQDN() string { return f.Git.TLSFQDN() }

// TLSHost returns just the host portion of the TLS FQDN (no port).
func (f *Fixture) TLSHost() string { return strings.SplitN(f.TLSFQDN(), ":", 2)[0] }

// AltFQDN returns host:port of the alternate git server HTTP listener.
func (f *Fixture) AltFQDN() string { return f.GitAlt.FQDN() }

// AltRepoURL returns the plain-HTTP clone URL for a repo on the alt server.
func (f *Fixture) AltRepoURL(repo syngitenvtest.RepoRef) string { return f.GitAlt.RepoURL(repo) }

// --- Repo management -------------------------------------------------------

// SecondRepo creates another repo on the primary server with the baseline
// permissions granted and returns its RepoRef.
func (f *Fixture) SecondRepo(suffix string) syngitenvtest.RepoRef {
	GinkgoHelper()
	id := seq.Add(1)
	repo := syngitenvtest.RepoRef{
		Owner: RepoOwner,
		Name:  fmt.Sprintf("repo-%d-%s", id, suffix),
	}
	Expect(f.Git.CreateRepo(repo, "main")).To(Succeed())
	grantBaseline(f.Git, repo)
	return repo
}

// AltRepo creates a repo on the alternate server with the baseline
// permissions granted and returns its RepoRef. Used by specs that need
// two distinct git hosts.
func (f *Fixture) AltRepo(suffix string) syngitenvtest.RepoRef {
	GinkgoHelper()
	id := seq.Add(1)
	repo := syngitenvtest.RepoRef{
		Owner: RepoOwner,
		Name:  fmt.Sprintf("repo-%d-%s", id, suffix),
	}
	Expect(f.GitAlt.CreateRepo(repo, "main")).To(Succeed())
	grantBaseline(f.GitAlt, repo)
	return repo
}

// CreateBranch creates branchName on the primary repo off sourceBranch.
func (f *Fixture) CreateBranch(branchName, sourceBranch string) {
	GinkgoHelper()
	Expect(f.Git.CreateBranch(f.Repo, branchName, sourceBranch)).To(Succeed())
}

// WaitForDynamicWebhook blocks until the controller has added a webhook
// entry for rsName on the shared ValidatingWebhookConfiguration, i.e.
// until the apiserver's admission chain actually routes intercepted
// resources to our in-process handler. Call this between creating a
// RemoteSyncer and attempting the first intercepted create/update/delete,
// otherwise the first request may land before the webhook is wired up
// and slip past interception.
func (f *Fixture) WaitForDynamicWebhook(rsName string) {
	GinkgoHelper()
	expectedName := rsName + "." + f.Namespace + ".syngit.io"
	Eventually(func() bool {
		vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{}
		err := f.Users.CtrlAs(Admin).Get(f.Ctx,
			types.NamespacedName{Name: DynamicWebhookName}, vwc)
		if err != nil {
			return false
		}
		for _, w := range vwc.Webhooks {
			if w.Name == expectedName {
				return true
			}
		}
		return false
	}).WithTimeout(DefaultTimeout).WithPolling(DefaultInterval).Should(BeTrue(),
		"dynamic webhook %q was never registered on %q", expectedName, DynamicWebhookName)
}

// WaitForDynamicWebhookToBeRemoved blocks until the controller has
// removed the webhook entry for rsName on the shared
// ValidatingWebhookConfiguration. Call this after deleting a RemoteSyncer.
func (f *Fixture) WaitForDynamicWebhookToBeRemoved(rsName string) {
	GinkgoHelper()
	expectedName := rsName + "." + f.Namespace + ".syngit.io"
	Eventually(func() bool {
		vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{}
		err := f.Users.CtrlAs(Admin).Get(f.Ctx,
			types.NamespacedName{Name: DynamicWebhookName}, vwc)
		if err != nil {
			return true
		}
		for _, w := range vwc.Webhooks {
			if w.Name == expectedName {
				return false
			}
		}
		return true
	}).WithTimeout(DefaultTimeout).WithPolling(DefaultInterval).Should(BeTrue(),
		"dynamic webhook %q was never removed on %q", expectedName, DynamicWebhookName)
}

// --- Object factories ------------------------------------------------------

// NewRemoteUser builds a RemoteUser object pointing at the primary git
// server's HTTP FQDN and the user's basic-auth secret. If managed is
// true, the syngit managed annotation is set so the controller creates a
// RemoteUserBinding.
func (f *Fixture) NewRemoteUser(user TestUser, name string, managed bool) *syngit.RemoteUser {
	ann := map[string]string{}
	if managed {
		ann[syngit.RubAnnotationKeyManaged] = "true"
	}
	return &syngit.RemoteUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   f.Namespace,
			Annotations: ann,
		},
		Spec: syngit.RemoteUserSpec{
			Email:             DefaultEmail(user),
			GitBaseDomainFQDN: f.FQDN(),
			SecretRef: corev1.SecretReference{
				Name: string(user) + "-creds",
			},
		},
	}
}

// NewTLSRemoteUser is NewRemoteUser but targets the TLS FQDN. Used by
// the TLS spec.
func (f *Fixture) NewTLSRemoteUser(user TestUser, name string, managed bool) *syngit.RemoteUser {
	ru := f.NewRemoteUser(user, name, managed)
	ru.Spec.GitBaseDomainFQDN = f.TLSFQDN()
	return ru
}

// NewCABundleSecret builds a kubernetes.io/tls secret in the fixture's
// namespace containing the primary GitServer's CA cert as tls.crt.
// Suitable for RemoteSyncer.Spec.CABundleSecretRef.
func (f *Fixture) NewCABundleSecret(name string) *corev1.Secret {
	return f.NewCABundleSecretInNamespace(name, f.Namespace)
}

// NewCABundleSecretInNamespace is NewCABundleSecret but places the secret
// in ns. Used for the operator-namespace discovery pattern.
func (f *Fixture) NewCABundleSecretInNamespace(name, ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       f.Git.CACert(),
			corev1.TLSPrivateKeyKey: {},
		},
	}
}
