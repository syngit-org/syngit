package refs

import (
	"testing"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ownerNs is the namespace of the referencing object in every case below.
const ownerNs = "owner-ns"

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
			ownerNamespace: ownerNs,
			want:           ownerNs,
		},
		{
			name:           "explicit ref namespace wins over the owner namespace",
			refNamespace:   "other-ns",
			ownerNamespace: ownerNs,
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
	refs, err := RemoteSyncerRefs(fullRemoteSyncerSpec(), ownerNs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []ObjectRef{
		{Namespace: ownerNs, Name: "default-ru", Group: "syngit.io", Version: "v1beta5", Resource: "remoteusers"},
		{Namespace: "rt-ns", Name: "default-rt", Group: "syngit.io", Version: "v1beta5", Resource: "remotetargets"},
		{Namespace: ownerNs, Name: "cm-local", Group: "", Version: "v1", Resource: "configmaps"},
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
			refs, err := RemoteSyncerRefs(tt.spec, ownerNs)
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

func TestRemoteUserRefs(t *testing.T) {
	t.Run("the secret ref is enumerated with its resolved namespace", func(t *testing.T) {
		refs, err := RemoteUserRefs(
			syngit.RemoteUserSpec{SecretRef: corev1.SecretReference{Name: "creds"}}, ownerNs,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(refs) != 1 {
			t.Fatalf("got %d refs, want 1: %+v", len(refs), refs)
		}
		got := refs[0]
		if got.Namespace != ownerNs || got.Name != "creds" ||
			got.Group != "" || got.Version != "v1" || got.Resource != "secrets" {
			t.Errorf("got %+v, want %s/creds as a core v1 secret", got, ownerNs)
		}
		if p := got.FieldPath.String(); p != "spec.secretRef" {
			t.Errorf("got field path %q, want spec.secretRef", p)
		}
	})

	t.Run("an explicit namespace wins", func(t *testing.T) {
		refs, err := RemoteUserRefs(syngit.RemoteUserSpec{
			SecretRef: corev1.SecretReference{Name: "creds", Namespace: "vault-ns"},
		}, ownerNs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(refs) != 1 || refs[0].Namespace != "vault-ns" {
			t.Fatalf("got %+v, want a single ref in vault-ns", refs)
		}
	})

	t.Run("an unset secret ref yields no ref", func(t *testing.T) {
		refs, err := RemoteUserRefs(syngit.RemoteUserSpec{}, ownerNs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(refs) != 0 {
			t.Errorf("got %d refs, want 0: %+v", len(refs), refs)
		}
	})
}

func TestRemoteUserBindingRefs(t *testing.T) {
	spec := syngit.RemoteUserBindingSpec{
		RemoteUserRefs: []corev1.ObjectReference{
			{Name: "ru-local"},
			{Name: "ru-remote", Namespace: "other-ns"},
		},
		RemoteTargetRefs: []corev1.ObjectReference{
			{Name: "rt-remote", Namespace: "target-ns"},
			{Name: "rt-local"},
		},
	}

	refs, err := RemoteUserBindingRefs(spec, ownerNs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []ObjectRef{
		{Namespace: ownerNs, Name: "ru-local", Group: "syngit.io", Version: "v1beta5", Resource: "remoteusers"},
		{Namespace: "other-ns", Name: "ru-remote", Group: "syngit.io", Version: "v1beta5", Resource: "remoteusers"},
		{Namespace: "target-ns", Name: "rt-remote", Group: "syngit.io", Version: "v1beta5", Resource: "remotetargets"},
		{Namespace: ownerNs, Name: "rt-local", Group: "syngit.io", Version: "v1beta5", Resource: "remotetargets"},
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
	}

	if p := refs[1].FieldPath.String(); p != "spec.remoteUserRefs[1]" {
		t.Errorf("got field path %q, want spec.remoteUserRefs[1]", p)
	}
	if p := refs[3].FieldPath.String(); p != "spec.remoteTargetRefs[1]" {
		t.Errorf("got field path %q, want spec.remoteTargetRefs[1]", p)
	}
}

// A namespace set on one reference must not bleed onto the following ones.
func TestRemoteUserBindingRefsDoesNotLeakNamespace(t *testing.T) {
	spec := syngit.RemoteUserBindingSpec{
		RemoteUserRefs: []corev1.ObjectReference{
			{Name: "first", Namespace: "other-ns"},
			{Name: "second"},
		},
		RemoteTargetRefs: []corev1.ObjectReference{{Name: "third"}},
	}

	refs, err := RemoteUserBindingRefs(spec, ownerNs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("got %d refs, want 3: %+v", len(refs), refs)
	}
	if refs[0].Namespace != "other-ns" {
		t.Errorf("first ref: got namespace %q, want other-ns", refs[0].Namespace)
	}
	if refs[1].Namespace != ownerNs {
		t.Errorf("second ref: got namespace %q, want owner-ns", refs[1].Namespace)
	}
	if refs[2].Namespace != ownerNs {
		t.Errorf("third ref: got namespace %q, want owner-ns", refs[2].Namespace)
	}
}
