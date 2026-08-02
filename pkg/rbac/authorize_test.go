package rbac

import (
	"context"
	"testing"

	"github.com/syngit-org/syngit/pkg/refs"
	authenticationv1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ownerNs is the namespace of the referencing object in every case below.
const ownerNs = "owner-ns"

// sarRecorder is a client that answers SubjectAccessReviews from a fixed allow-list
// and records every namespace it was asked about.
type sarRecorder struct {
	client.Client
	allowed map[string]bool // "<namespace>/<name>" -> allowed
	asked   []string
}

func (s *sarRecorder) Create(
	ctx context.Context, obj client.Object, opts ...client.CreateOption,
) error {
	sar, ok := obj.(*authv1.SubjectAccessReview)
	if !ok {
		return s.Client.Create(ctx, obj, opts...)
	}
	key := sar.Spec.ResourceAttributes.Namespace + "/" + sar.Spec.ResourceAttributes.Name
	s.asked = append(s.asked, key)
	sar.Status.Allowed = s.allowed[key]
	return nil
}

func newSARRecorder(allowed map[string]bool) *sarRecorder {
	return &sarRecorder{
		Client:  fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build(),
		allowed: allowed,
	}
}

func TestAuthorizeCrossNamespaceRefs(t *testing.T) {
	user := authenticationv1.UserInfo{Username: "alice", Groups: []string{"devs"}}
	objectRefs := []refs.ObjectRef{
		{Namespace: ownerNs, Name: "local", Resource: "configmaps", Version: "v1", FieldPath: field.NewPath("a")},
		{Namespace: "manager-ns", Name: "shared", Resource: "secrets", Version: "v1", FieldPath: field.NewPath("b")},
		{Namespace: "other-ns", Name: "remote", Resource: "secrets", Version: "v1", FieldPath: field.NewPath("c")},
	}

	t.Run("same-namespace and exempt refs are never checked", func(t *testing.T) {
		c := newSARRecorder(map[string]bool{"other-ns/remote": true})

		denied, err := AuthorizeCrossNamespaceRefs(
			context.Background(), c, user, objectRefs, ownerNs, "manager-ns",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if denied != nil {
			t.Fatalf("expected no denial, got %+v", denied)
		}
		if len(c.asked) != 1 || c.asked[0] != "other-ns/remote" {
			t.Errorf("expected exactly one SAR for other-ns/remote, got %v", c.asked)
		}
	})

	t.Run("a forbidden cross-namespace ref is returned", func(t *testing.T) {
		c := newSARRecorder(map[string]bool{"other-ns/remote": false})

		denied, err := AuthorizeCrossNamespaceRefs(
			context.Background(), c, user, objectRefs, ownerNs, "manager-ns",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if denied == nil {
			t.Fatal("expected a denial")
		}
		if denied.Name != "remote" || denied.Namespace != "other-ns" {
			t.Errorf("got denial for %s/%s, want other-ns/remote", denied.Namespace, denied.Name)
		}
	})

	t.Run("without an exemption the manager namespace is checked too", func(t *testing.T) {
		c := newSARRecorder(map[string]bool{"other-ns/remote": true, "manager-ns/shared": false})

		denied, err := AuthorizeCrossNamespaceRefs(
			context.Background(), c, user, objectRefs, ownerNs,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if denied == nil || denied.Namespace != "manager-ns" {
			t.Fatalf("expected a denial on manager-ns, got %+v", denied)
		}
	})

	t.Run("a cluster-scoped owner checks every ref", func(t *testing.T) {
		c := newSARRecorder(map[string]bool{
			"owner-ns/local": true, "manager-ns/shared": true, "other-ns/remote": true,
		})

		denied, err := AuthorizeCrossNamespaceRefs(context.Background(), c, user, objectRefs, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if denied != nil {
			t.Fatalf("expected no denial, got %+v", denied)
		}
		if len(c.asked) != 3 {
			t.Errorf("expected 3 SARs, got %v", c.asked)
		}
	})
}

// AuthorizeRefs is the counterpart of AuthorizeCrossNamespaceRefs: it must check
// every reference, including the ones resolving in the owner's own namespace.
// RemoteUser and RemoteUserBinding rely on that, because being allowed to write
// one of them must never imply being allowed to use what it points at.
func TestAuthorizeRefs(t *testing.T) {
	user := authenticationv1.UserInfo{Username: "alice", Groups: []string{"devs"}}
	objectRefs := []refs.ObjectRef{
		{Namespace: ownerNs, Name: "local", Resource: "secrets", Version: "v1", FieldPath: field.NewPath("a")},
		{Namespace: "other-ns", Name: "remote", Resource: "secrets", Version: "v1", FieldPath: field.NewPath("b")},
	}

	t.Run("every ref is checked, same namespace included", func(t *testing.T) {
		c := newSARRecorder(map[string]bool{"owner-ns/local": true, "other-ns/remote": true})

		denied, err := AuthorizeRefs(context.Background(), c, user, objectRefs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if denied != nil {
			t.Fatalf("expected no denial, got %+v", denied)
		}
		if len(c.asked) != 2 {
			t.Errorf("expected 2 SARs, got %v", c.asked)
		}
	})

	t.Run("a forbidden same-namespace ref is returned", func(t *testing.T) {
		c := newSARRecorder(map[string]bool{"owner-ns/local": false, "other-ns/remote": true})

		denied, err := AuthorizeRefs(context.Background(), c, user, objectRefs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if denied == nil {
			t.Fatal("expected a denial on the same-namespace ref")
		}
		if denied.Namespace != ownerNs || denied.Name != "local" {
			t.Errorf("got denial for %s/%s, want owner-ns/local", denied.Namespace, denied.Name)
		}
	})

	t.Run("no refs means nothing to check", func(t *testing.T) {
		c := newSARRecorder(nil)

		denied, err := AuthorizeRefs(context.Background(), c, user, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if denied != nil || len(c.asked) != 0 {
			t.Errorf("expected no denial and no SAR, got %+v / %v", denied, c.asked)
		}
	})
}

func TestCheckAccessPropagatesUserIdentity(t *testing.T) {
	c := newSARRecorder(map[string]bool{"ns/name": true})
	user := authenticationv1.UserInfo{
		Username: "alice",
		UID:      "uid-1",
		Groups:   []string{"devs", "system:authenticated"},
	}

	allowed, err := CheckAccess(context.Background(), c, user, authv1.ResourceAttributes{
		Namespace: "ns", Verb: "get", Version: "v1", Resource: "secrets", Name: "name",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected the access to be allowed")
	}
	if len(c.asked) != 1 || c.asked[0] != "ns/name" {
		t.Errorf("expected one SAR for ns/name, got %v", c.asked)
	}
}
