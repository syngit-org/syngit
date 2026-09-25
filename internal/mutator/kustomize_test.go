package mutator

import (
	"context"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	kustomizeprovider "github.com/syngit-org/syngit-provider-kustomize/pkg"
	"github.com/syngit-org/syngit/internal/walker"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	features "github.com/syngit-org/syngit/pkg/feature"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	kustomizeBundle    = "apps/web"
	kustomizeBasePath  = "apps/web/base/web.yaml"
	kustomizePatchPath = "apps/web/overlays/production/web.yaml"
)

// interceptedDeploymentYAML is the Deployment as `kustomize build` rendered it
// into the cluster: prefixed name, injected namespace and injected label.
const interceptedDeploymentYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: prod-web
  namespace: production
  labels:
    bundle: myapp
spec:
  replicas: 3
  template:
    metadata:
      labels:
        bundle: myapp
    spec:
      containers:
      - name: web
        image: web:2.0
`

// seedKustomizeBundle writes a base/overlay bundle whose overlay reaches its base
// through a relative resources entry.
func seedKustomizeBundle(t *testing.T, wt *git.Worktree) {
	t.Helper()

	files := map[string]string{
		"apps/web/base/kustomization.yaml": "resources:\n- web.yaml\n",
		kustomizeBasePath: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: web
        image: web:1.0
`,
		"apps/web/overlays/production/kustomization.yaml": `resources:
- ../../base
namePrefix: prod-
namespace: production
labels:
- pairs:
    bundle: myapp
`,
	}

	for path, content := range files {
		if err := walker.WriteWorktreeFile(wt, path, []byte(content)); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}
}

func kustomizeParams(annotations map[string]string, override kustomizeprovider.OverrideType) interceptor.GitPipelineParams {
	if annotations == nil {
		annotations = map[string]string{}
	}
	if _, ok := annotations[kustomizeprovider.BundlePathAnnotation]; !ok {
		annotations[kustomizeprovider.BundlePathAnnotation] = kustomizeBundle
	}
	annotations[kustomizeprovider.OverlayAnnotation] = "production"

	return interceptor.GitPipelineParams{
		Syncer: interceptor.SyncerContext{
			InterceptedNamespace: "production",
			Spec: syngit.RemoteSyncerSpec{
				Kustomize:      syngit.KustomizeConfig{Enabled: true, Override: override},
				ResourceFinder: true,
			},
		},
		InterceptedYAML:        interceptedDeploymentYAML,
		InterceptedGVR:         schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		InterceptedName:        "prod-web",
		InterceptedAnnotations: annotations,
	}
}

func renderKustomize(t *testing.T, wt *git.Worktree, params interceptor.GitPipelineParams) *ArtifactSet {
	t.Helper()

	out := &ArtifactSet{}
	rc := RenderContext{Ctx: context.Background(), Params: params, Worktree: wt}
	if err := (KustomizeProvider{}).Render(rc, out); err != nil {
		t.Fatalf("render: %v", err)
	}
	return out
}

func TestKustomizeProvider_OverlayPatchStripsInjectedFields(t *testing.T) {
	wt := newMemWorktree(t)
	seedKustomizeBundle(t, wt)

	out := renderKustomize(t, wt, kustomizeParams(nil, kustomizeprovider.Overlay))

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(out.Items))
	}
	artifact := out.Items[0]
	if artifact.TargetPath != kustomizePatchPath {
		t.Errorf("target path = %q, want %q", artifact.TargetPath, kustomizePatchPath)
	}

	patch := string(artifact.Content)
	if !strings.Contains(patch, "name: web") {
		t.Errorf("the patch must carry the base name, got:\n%s", patch)
	}
	if !strings.Contains(patch, "replicas: 3") || !strings.Contains(patch, "image: web:2.0") {
		t.Errorf("the patch must carry the difference against the base, got:\n%s", patch)
	}
	if strings.Contains(patch, "prod-web") || strings.Contains(patch, "namespace: production") ||
		strings.Contains(patch, "bundle: myapp") {
		t.Errorf("the patch must not carry what the overlay injects, got:\n%s", patch)
	}
}

func TestKustomizeProvider_BaseOverrideTargetsTheDetectedBase(t *testing.T) {
	wt := newMemWorktree(t)
	seedKustomizeBundle(t, wt)

	out := renderKustomize(t, wt, kustomizeParams(nil, kustomizeprovider.Base))

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(out.Items))
	}
	artifact := out.Items[0]
	if artifact.TargetPath != kustomizeBasePath {
		t.Errorf("target path = %q, want %q", artifact.TargetPath, kustomizeBasePath)
	}

	base := string(artifact.Content)
	if !strings.Contains(base, "kind: Deployment") || !strings.Contains(base, "name: web") {
		t.Errorf("the base must be the whole resource, got:\n%s", base)
	}
	if !strings.Contains(base, "image: web:2.0") {
		t.Errorf("the base must carry the intercepted state, got:\n%s", base)
	}
}

func TestKustomizeProvider_OverlayOnlyWritesTheWholeResourceInTheOverlay(t *testing.T) {
	wt := newMemWorktree(t)
	seedKustomizeBundle(t, wt)

	annotations := map[string]string{kustomizeprovider.OverlayOnlyAnnotation: "true"}
	out := renderKustomize(t, wt, kustomizeParams(annotations, kustomizeprovider.Overlay))

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(out.Items))
	}
	artifact := out.Items[0]
	if artifact.TargetPath != kustomizePatchPath {
		t.Errorf("target path = %q, want %q", artifact.TargetPath, kustomizePatchPath)
	}

	resource := string(artifact.Content)
	if !strings.Contains(resource, "kind: Deployment") || !strings.Contains(resource, "image: web:2.0") {
		t.Errorf("an overlay-only resource is written whole, got:\n%s", resource)
	}
	if strings.Contains(resource, "bundle: myapp") {
		t.Errorf("an overlay-only resource must still be stripped of what the overlay injects, got:\n%s", resource)
	}
}

func TestKustomizeProvider_DeclinesWithoutBundlePath(t *testing.T) {
	wt := newMemWorktree(t)
	seedKustomizeBundle(t, wt)

	params := kustomizeParams(map[string]string{kustomizeprovider.BundlePathAnnotation: ""}, kustomizeprovider.Overlay)
	params.InterceptedAnnotations[kustomizeprovider.BundlePathAnnotation] = ""

	out := renderKustomize(t, wt, params)

	if len(out.Items) != 0 {
		t.Fatalf("an object with no bundle-path is not kustomize-managed, got %d artifacts", len(out.Items))
	}
}

func TestKustomizeProvider_DeletionEmitsTheBundlePath(t *testing.T) {
	wt := newMemWorktree(t)
	seedKustomizeBundle(t, wt)

	params := kustomizeParams(nil, kustomizeprovider.Overlay)
	params.InterceptedYAML = ""

	out := renderKustomize(t, wt, params)

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(out.Items))
	}
	artifact := out.Items[0]
	if !artifact.IsDeletion() {
		t.Errorf("expected a deletion artifact, got %q", artifact.Content)
	}
	if artifact.TargetPath != kustomizePatchPath {
		t.Errorf("target path = %q, want %q", artifact.TargetPath, kustomizePatchPath)
	}
}

func TestKustomizeProvider_RejectsAnUnknownPatchStrategy(t *testing.T) {
	wt := newMemWorktree(t)
	seedKustomizeBundle(t, wt)

	annotations := map[string]string{kustomizeprovider.PatchStrategyAnnotation: "Rebase"}
	params := kustomizeParams(annotations, kustomizeprovider.Overlay)

	rc := RenderContext{Ctx: context.Background(), Params: params, Worktree: wt}
	if err := (KustomizeProvider{}).Render(rc, &ArtifactSet{}); err == nil {
		t.Fatal("expected an error for an unknown patch strategy")
	}
}

// A JSON6902 patch is a YAML sequence with no Kubernetes identity, so it cannot
// be located inside its own file. It must replace the revision it supersedes
// rather than be appended next to it.
func TestKustomizeProvider_JSON6902DoesNotAccumulate(t *testing.T) {
	wt := newMemWorktree(t)
	seedKustomizeBundle(t, wt)

	annotations := map[string]string{kustomizeprovider.PatchStrategyAnnotation: string(kustomizeprovider.JSON6902)}
	params := kustomizeParams(annotations, kustomizeprovider.Overlay)

	write := func() string {
		out := renderKustomize(t, wt, params)
		if len(out.Items) != 1 {
			t.Fatalf("expected 1 artifact, got %d", len(out.Items))
		}
		claimed := interceptor.NewClaimedPaths()
		if err := writeArtifactAtPath(wt, out.Items[0], nil, &claimed); err != nil {
			t.Fatalf("write artifact: %v", err)
		}
		return readWorktree(t, wt, kustomizePatchPath)
	}

	first := write()
	if second := write(); second != first {
		t.Errorf("pushing an unchanged resource twice must be idempotent, got:\n%s\nwant:\n%s", second, first)
	}
}

func TestKustomizeProvider_ResourceFinderReusesTheExistingFile(t *testing.T) {
	const existingPatch = "apps/web/overlays/production/tuning.yaml"

	wt := newMemWorktree(t)
	seedKustomizeBundle(t, wt)
	seeded := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 2
`
	if err := walker.WriteWorktreeFile(wt, existingPatch, []byte(seeded)); err != nil {
		t.Fatalf("seed %s: %v", existingPatch, err)
	}

	out := renderKustomize(t, wt, kustomizeParams(nil, kustomizeprovider.Overlay))

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(out.Items))
	}
	if out.Items[0].TargetPath != existingPatch {
		t.Errorf("target path = %q, want the file already holding the patch %q", out.Items[0].TargetPath, existingPatch)
	}
}

// The provider locates the files of a bundle by their identity, which is what
// the ResourceFinder does, so it only runs when the ResourceFinder does.
func TestKustomizeProvider_HandlesRequiresTheResourceFinder(t *testing.T) {
	previous := features.LoadedFeatureGates[features.ResourceFinder]
	t.Cleanup(func() { features.LoadedFeatureGates[features.ResourceFinder] = previous })

	params := kustomizeParams(nil, kustomizeprovider.Overlay)

	features.LoadedFeatureGates[features.ResourceFinder] = true
	if !(KustomizeProvider{}).Handles(params) {
		t.Error("a kustomize-enabled syncer with the ResourceFinder on must be handled")
	}

	features.LoadedFeatureGates[features.ResourceFinder] = false
	if (KustomizeProvider{}).Handles(params) {
		t.Error("the ResourceFinder feature gate off must decline")
	}

	features.LoadedFeatureGates[features.ResourceFinder] = true
	params.Syncer.Spec.ResourceFinder = false
	if (KustomizeProvider{}).Handles(params) {
		t.Error("spec.resourceFinder off must decline")
	}
}

// A bundle file may hold several resources. The base must be read as the single
// document carrying the identity, not as the whole file.
func TestKustomizeProvider_ReadsTheBaseOutOfAMultiDocumentFile(t *testing.T) {
	wt := newMemWorktree(t)
	seedKustomizeBundle(t, wt)

	multiDoc := `apiVersion: v1
kind: Service
metadata:
  name: web
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: web
        image: web:1.0
`
	if err := walker.WriteWorktreeFile(wt, kustomizeBasePath, []byte(multiDoc)); err != nil {
		t.Fatalf("seed %s: %v", kustomizeBasePath, err)
	}

	out := renderKustomize(t, wt, kustomizeParams(nil, kustomizeprovider.Overlay))

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(out.Items))
	}
	patch := string(out.Items[0].Content)
	if !strings.Contains(patch, "kind: Deployment") || !strings.Contains(patch, "image: web:2.0") {
		t.Errorf("the patch must differ against the Deployment document alone, got:\n%s", patch)
	}
}
