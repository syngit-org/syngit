package mutator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/go-git/go-git/v5"
	sopsprovider "github.com/syngit-org/syngit-provider-sops/pkg"
	"github.com/syngit-org/syngit/internal/walker"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	features "github.com/syngit-org/syngit/pkg/feature"
	"github.com/syngit-org/syngit/pkg/interceptor"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	sopsSecretName      = "sops-age"
	sopsSecretNamespace = "syngit"
	sopsOwnerNamespace  = "prod"
)

// enableSopsGate turns the alpha feature gate on for the duration of a test.
func enableSopsGate(t *testing.T) {
	t.Helper()
	previous := features.LoadedFeatureGates[features.SopsEncryption]
	features.LoadedFeatureGates[features.SopsEncryption] = true
	t.Cleanup(func() { features.LoadedFeatureGates[features.SopsEncryption] = previous })
}

// newAgeIdentity returns a throwaway age key pair, standing in for the identity
// a cluster would hold in a Secret.
func newAgeIdentity(t *testing.T) *age.X25519Identity {
	t.Helper()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate age identity: %v", err)
	}
	return identity
}

// seedSopsYAML writes a .sops.yaml carrying a single creation rule into the
// worktree, the way a repository using SOPS would.
func seedSopsYAML(t *testing.T, wt *git.Worktree, pathRegex, encryptedRegex, recipient string) {
	t.Helper()
	conf := fmt.Sprintf(`creation_rules:
  - path_regex: %s
    encrypted_regex: '%s'
    age: %s
`, pathRegex, encryptedRegex, recipient)
	if err := walker.WriteWorktreeFile(wt, sopsConfigFile, []byte(conf)); err != nil {
		t.Fatalf("seed %s: %v", sopsConfigFile, err)
	}
}

// sopsRenderContext builds a RenderContext for a syncer with SOPS enabled,
// backed by a fake cluster holding the age identity.
func sopsRenderContext(t *testing.T, wt *git.Worktree, identity *age.X25519Identity, withSecretRef bool) RenderContext {
	t.Helper()

	sops := syngit.SOPSConfig{Enabled: true}
	builder := fake.NewClientBuilder()
	if withSecretRef {
		sops.SecretRef = corev1.SecretReference{Name: sopsSecretName, Namespace: sopsSecretNamespace}
		builder = builder.WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: sopsSecretName, Namespace: sopsSecretNamespace},
			Data:       map[string][]byte{"age" + sopsprovider.AgeKeySecretSuffix: []byte(identity.String())},
		})
	}

	return RenderContext{
		Ctx:      context.Background(),
		Worktree: wt,
		Cluster:  client.Reader(builder.Build()),
		Params: interceptor.GitPipelineParams{
			Syncer: interceptor.SyncerContext{
				Spec:                 syngit.RemoteSyncerSpec{SOPS: sops},
				RefOwnerNamespace:    sopsOwnerNamespace,
				InterceptedNamespace: sopsOwnerNamespace,
			},
		},
	}
}

// plainSecret is the manifest syngit would have intercepted.
func plainSecret(password string) []byte {
	return []byte(fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: db
  namespace: prod
type: Opaque
stringData:
  username: admin
  password: %s
`, password))
}

const secretTargetPath = "prod/core/v1/secrets/db.yaml"

func TestSopsTransform_EncryptsMatchingPath(t *testing.T) {
	enableSopsGate(t)
	wt := newMemWorktree(t)
	identity := newAgeIdentity(t)
	seedSopsYAML(t, wt, ".*", "^(data|stringData)$", identity.Recipient().String())

	transform, err := sopsTransform(sopsRenderContext(t, wt, identity, true))
	if err != nil {
		t.Fatalf("sopsTransform: %v", err)
	}
	if transform == nil {
		t.Fatal("expected a transform, got nil")
	}

	out, err := transform(secretTargetPath, nil, plainSecret("hunter2"))
	if err != nil {
		t.Fatalf("transform: %v", err)
	}

	if !sopsprovider.IsSopsEncrypted(out) {
		t.Fatalf("expected a SOPS document, got:\n%s", out)
	}
	if strings.Contains(string(out), "hunter2") {
		t.Fatalf("the password survived in cleartext:\n%s", out)
	}
	// The identity must stay readable, otherwise syngit could never find this
	// document again.
	for _, want := range []string{"kind: Secret", "name: db"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("expected %q to remain in cleartext, got:\n%s", want, out)
		}
	}

	// It really is decryptable with the key the cluster holds.
	identities, err := sopsprovider.AgeIdentitiesFromSecret(&corev1.Secret{
		Data: map[string][]byte{"age" + sopsprovider.AgeKeySecretSuffix: []byte(identity.String())},
	})
	if err != nil {
		t.Fatalf("read identities: %v", err)
	}
	rules, err := sopsprovider.LoadCreationRule([]byte(fmt.Sprintf(
		"creation_rules:\n  - path_regex: .*\n    encrypted_regex: '^(data|stringData)$'\n    age: %s\n",
		identity.Recipient().String())), secretTargetPath)
	if err != nil {
		t.Fatalf("load rule: %v", err)
	}
	decrypted, err := sopsprovider.Decrypt(out, sopsprovider.Config{Rules: rules, Identities: identities})
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !strings.Contains(string(decrypted), "hunter2") {
		t.Errorf("decrypted document lost the password:\n%s", decrypted)
	}
}

// A path outside every creation rule is the user scoping the document out of
// their encryption, not a misconfiguration: it must pass through untouched.
func TestSopsTransform_NonMatchingPathPassesThrough(t *testing.T) {
	enableSopsGate(t)
	wt := newMemWorktree(t)
	identity := newAgeIdentity(t)
	seedSopsYAML(t, wt, "^secrets/.*", "^(data|stringData)$", identity.Recipient().String())

	transform, err := sopsTransform(sopsRenderContext(t, wt, identity, true))
	if err != nil {
		t.Fatalf("sopsTransform: %v", err)
	}

	in := plainSecret("hunter2")
	out, err := transform("configmaps/app.yaml", nil, in)
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if string(out) != string(in) {
		t.Errorf("expected the document to pass through unchanged, got:\n%s", out)
	}
}

// Re-applying an unchanged object must reuse the ciphertext already committed,
// so that git sees no diff and the pipeline produces no commit.
func TestSopsTransform_UnchangedObjectKeepsCiphertext(t *testing.T) {
	enableSopsGate(t)
	wt := newMemWorktree(t)
	identity := newAgeIdentity(t)
	seedSopsYAML(t, wt, ".*", "^(data|stringData)$", identity.Recipient().String())

	transform, err := sopsTransform(sopsRenderContext(t, wt, identity, true))
	if err != nil {
		t.Fatalf("sopsTransform: %v", err)
	}

	first, err := transform(secretTargetPath, nil, plainSecret("hunter2"))
	if err != nil {
		t.Fatalf("first transform: %v", err)
	}

	second, err := transform(secretTargetPath, first, plainSecret("hunter2"))
	if err != nil {
		t.Fatalf("second transform: %v", err)
	}

	if string(second) != string(first) {
		t.Errorf("an unchanged object rewrote the document:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// Rotating a value must produce a different document, so the change is actually
// committed.
func TestSopsTransform_ChangedValueIsRewritten(t *testing.T) {
	enableSopsGate(t)
	wt := newMemWorktree(t)
	identity := newAgeIdentity(t)
	seedSopsYAML(t, wt, ".*", "^(data|stringData)$", identity.Recipient().String())

	transform, err := sopsTransform(sopsRenderContext(t, wt, identity, true))
	if err != nil {
		t.Fatalf("sopsTransform: %v", err)
	}

	first, err := transform(secretTargetPath, nil, plainSecret("hunter2"))
	if err != nil {
		t.Fatalf("first transform: %v", err)
	}
	rotated, err := transform(secretTargetPath, first, plainSecret("correct-horse"))
	if err != nil {
		t.Fatalf("rotated transform: %v", err)
	}

	if string(rotated) == string(first) {
		t.Error("rotating the password produced an identical document")
	}
	if strings.Contains(string(rotated), "correct-horse") {
		t.Errorf("the rotated password survived in cleartext:\n%s", rotated)
	}
}

// An encrypted_regex covering the metadata would hide the object's identity,
// which is what syngit locates documents by.
func TestSopsTransform_RejectsHiddenKubernetesIdentity(t *testing.T) {
	enableSopsGate(t)
	wt := newMemWorktree(t)
	identity := newAgeIdentity(t)
	seedSopsYAML(t, wt, ".*", "^(data|stringData|metadata|kind|apiVersion)$", identity.Recipient().String())

	transform, err := sopsTransform(sopsRenderContext(t, wt, identity, true))
	if err != nil {
		t.Fatalf("sopsTransform: %v", err)
	}

	_, err = transform(secretTargetPath, nil, plainSecret("hunter2"))
	if err == nil {
		t.Fatal("expected an error when the object identity is encrypted")
	}
	if !strings.Contains(err.Error(), "encrypted_regex") {
		t.Errorf("expected the error to point at encrypted_regex, got: %v", err)
	}
}

// enabled: true against a repository with no .sops.yaml is broken configuration,
// not a scoping decision: it must fail rather than push cleartext.
func TestSopsTransform_MissingSopsYAMLIsAnError(t *testing.T) {
	enableSopsGate(t)
	wt := newMemWorktree(t)
	identity := newAgeIdentity(t)

	_, err := sopsTransform(sopsRenderContext(t, wt, identity, true))
	if err == nil {
		t.Fatal("expected an error when .sops.yaml is missing")
	}
	if !strings.Contains(err.Error(), sopsConfigFile) {
		t.Errorf("expected the error to name %s, got: %v", sopsConfigFile, err)
	}
}

// Without a secret reference the provider still encrypts, from the .sops.yaml
// recipients alone; it just cannot reuse the existing ciphertext.
func TestSopsTransform_EncryptsWithoutSecretRef(t *testing.T) {
	enableSopsGate(t)
	wt := newMemWorktree(t)
	identity := newAgeIdentity(t)
	seedSopsYAML(t, wt, ".*", "^(data|stringData)$", identity.Recipient().String())

	transform, err := sopsTransform(sopsRenderContext(t, wt, identity, false))
	if err != nil {
		t.Fatalf("sopsTransform: %v", err)
	}

	out, err := transform(secretTargetPath, nil, plainSecret("hunter2"))
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if !sopsprovider.IsSopsEncrypted(out) {
		t.Fatalf("expected a SOPS document, got:\n%s", out)
	}
}

// The transform is only useful if the placement phase actually calls it, so
// drive the whole worktree generation and read the committed file back.
func TestGenerateFinalWorktree_EncryptsWithSops(t *testing.T) {
	enableSopsGate(t)
	wt := newMemWorktree(t)
	identity := newAgeIdentity(t)
	seedSopsYAML(t, wt, ".*", "^(data|stringData)$", identity.Recipient().String())

	rc := sopsRenderContext(t, wt, identity, true)
	params := rc.Params
	params.InterceptedYAML = string(plainSecret("hunter2"))
	params.InterceptedName = "db"
	params.InterceptedGVR = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

	_, claimed, err := GenerateFinalWorktree(rc.Ctx, rc.Cluster, params, wt)
	if err != nil {
		t.Fatalf("GenerateFinalWorktree: %v", err)
	}
	if len(claimed.Add) != 1 {
		t.Fatalf("got %d claimed paths, want 1: %+v", len(claimed.Add), claimed.Add)
	}

	written := readWorktree(t, wt, claimed.Add[0])
	if !sopsprovider.IsSopsEncrypted([]byte(written)) {
		t.Fatalf("the placement phase wrote a cleartext document at %s:\n%s", claimed.Add[0], written)
	}
	if strings.Contains(written, "hunter2") {
		t.Errorf("the password reached the worktree in cleartext:\n%s", written)
	}
}

// A misconfigured SOPS setup must fail the pipeline before anything lands in
// the worktree, rather than falling back to a cleartext push.
func TestGenerateFinalWorktree_FailsWithoutSopsYAML(t *testing.T) {
	enableSopsGate(t)
	wt := newMemWorktree(t)
	identity := newAgeIdentity(t)

	rc := sopsRenderContext(t, wt, identity, true)
	params := rc.Params
	params.InterceptedYAML = string(plainSecret("hunter2"))
	params.InterceptedName = "db"
	params.InterceptedGVR = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

	_, claimed, err := GenerateFinalWorktree(rc.Ctx, rc.Cluster, params, wt)
	if err == nil {
		t.Fatal("expected the pipeline to fail when .sops.yaml is missing")
	}
	if claimed.ClaimExists() {
		t.Errorf("nothing should have been claimed, got %+v", claimed)
	}
}

// The transform must stay off unless both the feature gate and the spec field
// are set.
func TestSopsTransform_Disabled(t *testing.T) {
	wt := newMemWorktree(t)
	identity := newAgeIdentity(t)
	seedSopsYAML(t, wt, ".*", "^(data|stringData)$", identity.Recipient().String())

	t.Run("gate off", func(t *testing.T) {
		transform, err := sopsTransform(sopsRenderContext(t, wt, identity, true))
		if err != nil {
			t.Fatalf("sopsTransform: %v", err)
		}
		if transform != nil {
			t.Error("expected no transform when the feature gate is off")
		}
	})

	t.Run("spec off", func(t *testing.T) {
		enableSopsGate(t)
		rc := sopsRenderContext(t, wt, identity, true)
		rc.Params.Syncer.Spec.SOPS.Enabled = false
		transform, err := sopsTransform(rc)
		if err != nil {
			t.Fatalf("sopsTransform: %v", err)
		}
		if transform != nil {
			t.Error("expected no transform when spec.sops.enabled is false")
		}
	})
}
