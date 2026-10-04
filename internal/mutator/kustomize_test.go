package mutator

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	kustomizeprovider "github.com/syngit-org/syngit-provider-kustomize/pkg"
	"github.com/syngit-org/syngit/internal/walker"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	features "github.com/syngit-org/syngit/pkg/feature"
	"github.com/syngit-org/syngit/pkg/interceptor"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	overlayDir           = "apps/my-app/overlays/staging"
	overlayKustomization = overlayDir + "/kustomization.yaml"
)

func enableKustomizeGates(t *testing.T) {
	t.Helper()
	for _, gate := range []features.Feature{features.Kustomize, features.ResourceFinder} {
		previous := features.LoadedFeatureGates[gate]
		features.LoadedFeatureGates[gate] = true
		t.Cleanup(func() { features.LoadedFeatureGates[gate] = previous })
	}
}

func seedKustomizeBundle(t *testing.T) *git.Worktree {
	t.Helper()
	wt := newMemWorktree(t)
	files := map[string]string{
		"apps/my-app/base/kustomization.yaml": `resources:
  - deployment.yaml
`,
		"apps/my-app/base/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 1
  selector:
    matchLabels: {app: my-app}
  template:
    metadata:
      labels: {app: my-app}
    spec:
      containers:
        - name: app
          image: nginx:1.27
`,
		overlayDir + "/debug-service.yaml": `apiVersion: v1
kind: Service
metadata:
  name: debug
spec:
  ports:
    - port: 80
`,
		overlayKustomization: `namePrefix: staging-
resources:
  - ../../base
  # kept by the edits
  - debug-service.yaml
labels:
  - pairs:
      kustomize.syngit.io/bundle: my-app
      kustomize.syngit.io/overlay: staging
commonAnnotations:
  kustomize.syngit.io/root: apps/my-app
`,
	}
	for path, content := range files {
		if err := walker.WriteWorktreeFile(wt, path, []byte(content)); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}
	return wt
}

func kustomizeParams(t *testing.T, op admissionv1.Operation, manifest string) interceptor.GitPipelineParams {
	t.Helper()
	params := interceptor.GitPipelineParams{
		Syncer: interceptor.SyncerContext{
			Annotations:          map[string]string{kustomizeprovider.ProviderAnnotation: "enabled"},
			InterceptedNamespace: "default",
		},
		Operation: op,
	}
	params.Syncer.Spec.ResourceFinder = true

	if op == admissionv1.Delete {
		params.DeletedManifest = manifest
	} else {
		params.InterceptedManifest = manifest
	}

	sel := walker.SelectorFromDoc([]byte(manifest))
	params.InterceptedGVR = schema.GroupVersionResource{Group: sel.GVR.Group, Version: "v1", Resource: sel.GVR.Resource}
	params.InterceptedName = sel.Name
	return params
}

func liveDeployment(replicas string) string {
	return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: staging-my-app
  namespace: default
  labels:
    kustomize.syngit.io/bundle: my-app
    kustomize.syngit.io/overlay: staging
  annotations:
    kustomize.syngit.io/root: apps/my-app
spec:
  replicas: ` + replicas + `
  selector:
    matchLabels: {app: my-app}
  template:
    metadata:
      labels: {app: my-app}
      annotations:
        kustomize.syngit.io/root: apps/my-app
    spec:
      containers:
        - name: app
          image: nginx:1.27
`
}

const liveDebugService = `apiVersion: v1
kind: Service
metadata:
  name: staging-debug
  namespace: default
  labels:
    kustomize.syngit.io/bundle: my-app
    kustomize.syngit.io/overlay: staging
  annotations:
    kustomize.syngit.io/root: apps/my-app
spec:
  ports:
    - port: 80
`

func TestKustomizeProvider_Handles(t *testing.T) {
	params := kustomizeParams(t, admissionv1.Update, liveDeployment("3"))
	if !(KustomizeProvider{}).Handles(params) {
		t.Fatal("expected the provider to handle a syncer carrying the annotation")
	}
	params.Syncer.Annotations = nil
	if (KustomizeProvider{}).Handles(params) {
		t.Fatal("expected the provider not to handle a syncer without the annotation")
	}
}

func TestGenerateFinalWorktree_KustomizePatchesBaseObject(t *testing.T) {
	enableKustomizeGates(t)
	wt := seedKustomizeBundle(t)

	_, claimed, err := GenerateFinalWorktree(context.Background(), nil, kustomizeParams(t, admissionv1.Update, liveDeployment("3")), wt)
	if err != nil {
		t.Fatalf("GenerateFinalWorktree: %v", err)
	}

	patchPath := overlayDir + "/deployment-my-app.patch.yaml"
	if !slices.Equal(claimed.Add, []string{patchPath, overlayKustomization}) && !slices.Equal(claimed.Add, []string{overlayKustomization, patchPath}) {
		t.Fatalf("claimed %v, want the patch and the kustomization", claimed.Add)
	}
	if patch := readWorktree(t, wt, patchPath); !strings.Contains(patch, "replicas: 3") || !strings.Contains(patch, "name: my-app") {
		t.Fatalf("unexpected patch:\n%s", patch)
	}
	kustomization := readWorktree(t, wt, overlayKustomization)
	if !strings.Contains(kustomization, "  - path: deployment-my-app.patch.yaml") {
		t.Fatalf("patch entry missing from the kustomization:\n%s", kustomization)
	}
	if !strings.Contains(kustomization, "  - ../../base\n  # kept by the edits\n") {
		t.Fatalf("comment or indentation lost in the kustomization:\n%s", kustomization)
	}
}

func TestGenerateFinalWorktree_KustomizeAddsOverlayResource(t *testing.T) {
	enableKustomizeGates(t)
	wt := seedKustomizeBundle(t)
	manifest := strings.Replace(strings.ReplaceAll(liveDebugService, "debug", "extra"),
		"annotations:\n", "annotations:\n    kustomize.syngit.io/overlay-only: \"true\"\n", 1)

	_, _, err := GenerateFinalWorktree(context.Background(), nil, kustomizeParams(t, admissionv1.Create, manifest), wt)
	if err != nil {
		t.Fatalf("GenerateFinalWorktree: %v", err)
	}

	if resource := readWorktree(t, wt, overlayDir+"/service-extra.yaml"); !strings.Contains(resource, "name: extra") {
		t.Fatalf("unexpected resource:\n%s", resource)
	}
	if kustomization := readWorktree(t, wt, overlayKustomization); !strings.Contains(kustomization, "- service-extra.yaml") {
		t.Fatalf("resource entry missing from the kustomization:\n%s", kustomization)
	}
}

func TestGenerateFinalWorktree_KustomizeDeletesOverlayResource(t *testing.T) {
	enableKustomizeGates(t)
	wt := seedKustomizeBundle(t)

	_, claimed, err := GenerateFinalWorktree(context.Background(), nil, kustomizeParams(t, admissionv1.Delete, liveDebugService), wt)
	if err != nil {
		t.Fatalf("GenerateFinalWorktree: %v", err)
	}

	if !slices.Equal(claimed.Delete, []string{overlayDir + "/debug-service.yaml"}) {
		t.Fatalf("claimed deletions %v, want the overlay resource", claimed.Delete)
	}
	if _, err := wt.Filesystem.Stat(overlayDir + "/debug-service.yaml"); err == nil {
		t.Fatal("expected the overlay resource file to be removed")
	}
	kustomization := readWorktree(t, wt, overlayKustomization)
	if strings.Contains(kustomization, "debug-service.yaml") {
		t.Fatalf("resource entry left in the kustomization:\n%s", kustomization)
	}
	if !strings.Contains(kustomization, "- ../../base") {
		t.Fatalf("other resource entry removed from the kustomization:\n%s", kustomization)
	}
}

func TestGenerateFinalWorktree_KustomizeNoOpWritesNothing(t *testing.T) {
	enableKustomizeGates(t)
	wt := seedKustomizeBundle(t)

	_, claimed, err := GenerateFinalWorktree(context.Background(), nil, kustomizeParams(t, admissionv1.Update, liveDeployment("1")), wt)
	if err != nil {
		t.Fatalf("GenerateFinalWorktree: %v", err)
	}
	if claimed.ClaimExists() {
		t.Fatalf("expected nothing written, claimed %v", claimed)
	}
}

func TestGenerateFinalWorktree_KustomizeRefusalDenies(t *testing.T) {
	enableKustomizeGates(t)
	wt := seedKustomizeBundle(t)
	manifest := strings.ReplaceAll(liveDebugService, "debug", "unknown")

	_, claimed, err := GenerateFinalWorktree(context.Background(), nil, kustomizeParams(t, admissionv1.Create, manifest), wt)
	if !errors.Is(err, syngiterrors.ErrProviderDenied) {
		t.Fatalf("expected a provider denial, got %v", err)
	}
	if !strings.Contains(err.Error(), kustomizeprovider.OverlayOnlyAnnotation) {
		t.Fatalf("expected the refusal message, got %v", err)
	}
	if claimed.ClaimExists() {
		t.Fatalf("expected nothing written, claimed %v", claimed)
	}
}

func TestGenerateFinalWorktree_KustomizeIgnoresObjectOutsideBundle(t *testing.T) {
	enableKustomizeGates(t)
	wt := seedKustomizeBundle(t)
	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: standalone
  namespace: default
`

	_, claimed, err := GenerateFinalWorktree(context.Background(), nil, kustomizeParams(t, admissionv1.Create, manifest), wt)
	if err != nil {
		t.Fatalf("GenerateFinalWorktree: %v", err)
	}
	if len(claimed.Add) != 1 || strings.HasPrefix(claimed.Add[0], "apps/") {
		t.Fatalf("expected the default placement, claimed %v", claimed.Add)
	}
}
