package utils

import (
	"context"
	"testing"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	authenticationv1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveNamespace(t *testing.T) {
	path := field.NewPath("spec", "someRef")

	tests := []struct {
		name           string
		refNamespace   string
		ownerNamespace string
		want           string
		wantErr        bool
	}{
		{
			name:           "empty ref namespace falls back to the owner namespace",
			refNamespace:   "",
			ownerNamespace: "owner-ns",
			want:           "owner-ns",
		},
		{
			name:           "explicit ref namespace wins over the owner namespace",
			refNamespace:   "other-ns",
			ownerNamespace: "owner-ns",
			want:           "other-ns",
		},
		{
			name:           "explicit ref namespace is enough for a cluster-scoped owner",
			refNamespace:   "other-ns",
			ownerNamespace: "",
			want:           "other-ns",
		},
		{
			name:           "cluster-scoped owner with no ref namespace is an error",
			refNamespace:   "",
			ownerNamespace: "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveNamespace(tt.refNamespace, tt.ownerNamespace, path)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got namespace %q", got)
				}
				if !syngiterrors.Is(err, syngiterrors.ErrMissingRefNamespace) {
					t.Fatalf("expected ErrMissingRefNamespace, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got namespace %q, want %q", got, tt.want)
			}
		})
	}
}

func fullRemoteSyncerSpec() syngit.RemoteSyncerSpec {
	return syngit.RemoteSyncerSpec{
		DefaultRemoteUserRef:   &corev1.ObjectReference{Name: "default-ru"},
		DefaultRemoteTargetRef: &corev1.ObjectReference{Name: "default-rt", Namespace: "rt-ns"},
		ExcludedFieldsConfigMapsRef: []*corev1.ObjectReference{
			{Name: "cm-local"},
			{Name: "cm-remote", Namespace: "cm-ns"},
		},
		CABundleSecretRef: corev1.SecretReference{Name: "ca", Namespace: "ca-ns"},
	}
}

func TestRemoteSyncerRefs(t *testing.T) {
	refs, err := RemoteSyncerRefs(fullRemoteSyncerSpec(), "owner-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []ObjectRef{
		{Namespace: "owner-ns", Name: "default-ru", Group: "syngit.io", Version: "v1beta5", Resource: "remoteusers"},
		{Namespace: "rt-ns", Name: "default-rt", Group: "syngit.io", Version: "v1beta5", Resource: "remotetargets"},
		{Namespace: "owner-ns", Name: "cm-local", Group: "", Version: "v1", Resource: "configmaps"},
		{Namespace: "cm-ns", Name: "cm-remote", Group: "", Version: "v1", Resource: "configmaps"},
		{Namespace: "ca-ns", Name: "ca", Group: "", Version: "v1", Resource: "secrets"},
	}

	if len(refs) != len(want) {
		t.Fatalf("got %d refs, want %d: %+v", len(refs), len(want), refs)
	}
	for i, w := range want {
		got := refs[i]
		if got.Namespace != w.Namespace || got.Name != w.Name ||
			got.Group != w.Group || got.Version != w.Version || got.Resource != w.Resource {
			t.Errorf("ref %d: got %+v, want %+v", i, got, w)
		}
		if got.FieldPath == nil {
			t.Errorf("ref %d (%s): field path is nil", i, got.Name)
		}
	}

	// The indexed ConfigMap path must identify which entry of the slice is at fault.
	if p := refs[3].FieldPath.String(); p != "spec.excludedFieldsConfigMapsRef[1]" {
		t.Errorf("got configmap field path %q, want spec.excludedFieldsConfigMapsRef[1]", p)
	}
}

func TestRemoteSyncerRefsSkipsUnsetRefs(t *testing.T) {
	tests := []struct {
		name string
		spec syngit.RemoteSyncerSpec
	}{
		{
			name: "empty spec yields no refs",
			spec: syngit.RemoteSyncerSpec{},
		},
		{
			name: "nil entry in the configmap slice is skipped",
			spec: syngit.RemoteSyncerSpec{ExcludedFieldsConfigMapsRef: []*corev1.ObjectReference{nil}},
		},
		{
			name: "refs with an empty name are skipped",
			spec: syngit.RemoteSyncerSpec{
				DefaultRemoteUserRef:   &corev1.ObjectReference{Namespace: "ns"},
				DefaultRemoteTargetRef: &corev1.ObjectReference{},
				CABundleSecretRef:      corev1.SecretReference{Namespace: "ns"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, err := RemoteSyncerRefs(tt.spec, "owner-ns")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(refs) != 0 {
				t.Errorf("got %d refs, want 0: %+v", len(refs), refs)
			}
		})
	}
}

// A cluster-scoped owner has no namespace to fall back to, so any reference
// without an explicit namespace must be rejected.
func TestRemoteSyncerRefsClusterScopedOwner(t *testing.T) {
	_, err := RemoteSyncerRefs(fullRemoteSyncerSpec(), "")
	if err == nil {
		t.Fatal("expected an error for a cluster-scoped owner with a namespace-less ref")
	}
	if !syngiterrors.Is(err, syngiterrors.ErrMissingRefNamespace) {
		t.Fatalf("expected ErrMissingRefNamespace, got %v", err)
	}

	// Fully qualified refs resolve fine without an owner namespace.
	spec := syngit.RemoteSyncerSpec{
		DefaultRemoteUserRef:   &corev1.ObjectReference{Name: "ru", Namespace: "ru-ns"},
		DefaultRemoteTargetRef: &corev1.ObjectReference{Name: "rt", Namespace: "rt-ns"},
		CABundleSecretRef:      corev1.SecretReference{Name: "ca", Namespace: "ca-ns"},
	}
	refs, err := RemoteSyncerRefs(spec, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("got %d refs, want 3", len(refs))
	}
}

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
	refs := []ObjectRef{
		{Namespace: "owner-ns", Name: "local", Resource: "configmaps", Version: "v1", FieldPath: field.NewPath("a")},
		{Namespace: "manager-ns", Name: "shared", Resource: "secrets", Version: "v1", FieldPath: field.NewPath("b")},
		{Namespace: "other-ns", Name: "remote", Resource: "secrets", Version: "v1", FieldPath: field.NewPath("c")},
	}

	t.Run("same-namespace and exempt refs are never checked", func(t *testing.T) {
		c := newSARRecorder(map[string]bool{"other-ns/remote": true})

		denied, err := AuthorizeCrossNamespaceRefs(
			context.Background(), c, user, refs, "owner-ns", "manager-ns",
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
			context.Background(), c, user, refs, "owner-ns", "manager-ns",
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
			context.Background(), c, user, refs, "owner-ns",
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

		denied, err := AuthorizeCrossNamespaceRefs(context.Background(), c, user, refs, "")
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
