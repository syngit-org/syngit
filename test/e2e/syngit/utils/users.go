package utils

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	syngitutils "github.com/syngit-org/syngit/pkg/utils"
)

// TestUser is the impersonated Kubernetes identity. The same string is
// reused as the git username registered on the envtest GitServers so
// K8s-impersonation and git-auth share one identity per role.
type TestUser string

// The three canonical test users. Their string values double as git
// usernames. The git password and email are derived from DefaultPassword
// and DefaultEmail.
const (
	// Admin is a cluster-admin. Impersonated with the system:masters group,
	// which bypasses RBAC in envtest.
	Admin TestUser = "admin"
	// Developer is the happy-path DevOps user. Granted cluster-admin via
	// a ClusterRoleBinding.
	Developer TestUser = "developer"
	// Restricted is the constrained RBAC user used for permission-denial
	// scenarios on the Kubernetes side. Bound to a narrow ClusterRole.
	// Git-side, Restricted has ReadWrite on every repo so tests about
	// Kubernetes RBAC (create RemoteSyncer, access secrets, etc.) still
	// exercise successful git pushes. For "no git access" scenarios
	// tests should point a RemoteUser at a bogus-credentials secret via
	// Fixture.NewBogusCredsSecret.
	Restricted TestUser = "restricted"
)

// AllUsers is the ordered list of canonical users. Fixtures iterate it
// to create per-namespace basic-auth secrets.
var AllUsers = []TestUser{Admin, Developer, Restricted}

// DefaultPassword returns the password registered on the git server for user.
func DefaultPassword(user TestUser) string { return string(user) + "-pwd" }

// DefaultEmail returns the email used when creating RemoteUsers for user.
func DefaultEmail(user TestUser) string { return string(user) + "@syngit.io" }

// SanitizeUser returns the sanitized form of the username matching what
// the syngit managed-RUB webhook appends to the RUB name.
func SanitizeUser(user TestUser) string { return syngitutils.Sanitize(string(user)) }

// UserClient is an impersonation-aware wrapper around a rest.Config.
// Methods return fresh clients each call so callers never share the
// rest.Config's Impersonate field.
type UserClient struct {
	baseConfig *rest.Config
}

// NewUserClient wraps cfg. Callers should pass the envtest rest.Config.
func NewUserClient(cfg *rest.Config) *UserClient { return &UserClient{baseConfig: cfg} }

// cfgAs returns a copy of the base rest.Config with Impersonate set for user.
func (u *UserClient) cfgAs(user TestUser) *rest.Config {
	cfg := rest.CopyConfig(u.baseConfig)
	groups := []string{"system:authenticated"}
	if user == Admin {
		groups = append(groups, "system:masters")
	}
	cfg.Impersonate = rest.ImpersonationConfig{
		UserName: string(user),
		Groups:   groups,
	}
	return cfg
}

// CtrlAs returns a controller-runtime client impersonating user.
func (u *UserClient) CtrlAs(user TestUser) client.Client {
	c, err := client.New(u.cfgAs(user), client.Options{Scheme: scheme.Scheme})
	if err != nil {
		panic(err)
	}
	return c
}

// KAs returns a typed Kubernetes clientset impersonating user.
func (u *UserClient) KAs(user TestUser) *k8s.Clientset {
	c, err := k8s.NewForConfig(u.cfgAs(user))
	if err != nil {
		panic(err)
	}
	return c
}

// CreateOrUpdate performs create-if-missing-else-update as user, mirroring
// the legacy test/utils CustomClient helper. Retries on conflict errors with
// exponential backoff to handle concurrent modifications from controllers.
func (u *UserClient) CreateOrUpdate(ctx context.Context, user TestUser, obj client.Object) error {
	c := u.CtrlAs(user)
	existing := obj.DeepCopyObject().(client.Object)
	err := c.Get(ctx, client.ObjectKeyFromObject(obj), existing)
	if errors.IsNotFound(err) {
		return c.Create(ctx, obj)
	}
	if err != nil {
		return err
	}

	err = retry.RetryOnConflict(wait.Backoff{
		Steps:    5,
		Duration: 10 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
	}, func() error {
		// Get fresh resource version before each attempt
		fresh := obj.DeepCopyObject().(client.Object)
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), fresh); err != nil {
			return err
		}
		obj.SetResourceVersion(fresh.GetResourceVersion())
		return c.Update(ctx, obj)
	})

	return err
}
