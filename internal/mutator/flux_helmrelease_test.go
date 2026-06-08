package mutator

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"github.com/syngit-org/syngit/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func newMemWorktree(t *testing.T) *git.Worktree {
	t.Helper()
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	return wt
}

// fakeClientCtx returns a context carrying a fake k8s client, as the webhook
// handler does at runtime. The provider's excluded-fields cleaning reads the
// client from the context (utils.K8sClientFromContext), which panics otherwise.
func fakeClientCtx(t *testing.T) context.Context {
	t.Helper()
	c := fake.NewClientBuilder().Build()
	return context.WithValue(context.Background(), utils.K8sClientCtxKey{}, client.Client(c))
}

// helmReleaseSecretYAML builds the YAML of a Helm release Secret
// (sh.helm.release.v1.podinfo.v1) carrying a gzip+base64 release blob for a
// chart named "podinfo", as Helm 3 stores it in the cluster.
func helmReleaseSecretYAML(t *testing.T) string {
	t.Helper()

	release := map[string]interface{}{
		"name":      "podinfo",
		"namespace": "default",
		"chart": map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":    "podinfo",
				"version": "6.5.0",
			},
		},
		"config": map[string]interface{}{
			"replicaCount": 2,
		},
	}
	jsonData, err := json.Marshal(release)
	if err != nil {
		t.Fatalf("marshal release: %v", err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(jsonData); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sh.helm.release.v1.podinfo.v1",
			Namespace: "default",
			Labels: map[string]string{
				"owner":   "helm",
				"name":    "podinfo",
				"status":  "deployed",
				"version": "1",
			},
		},
		Type: corev1.SecretType("helm.sh/release.v1"),
		Data: map[string][]byte{"release": []byte(encoded)},
	}

	raw, err := yaml.Marshal(secret)
	if err != nil {
		t.Fatalf("marshal secret: %v", err)
	}
	return string(raw)
}

func helmReleaseParams(t *testing.T) interceptor.GitPipelineParams {
	return interceptor.GitPipelineParams{
		RemoteSyncer: syngit.RemoteSyncer{
			Spec: syngit.RemoteSyncerSpec{
				// Server-managed fields that must not leak into the git manifest.
				ExcludedFields: []string{
					".metadata.uid",
					".metadata.resourceVersion",
					".metadata.creationTimestamp",
					".metadata.managedFields",
					".status",
				},
			},
		},
		InterceptedGVR: schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "secrets",
		},
		InterceptedName: "sh.helm.release.v1.podinfo.v1",
		InterceptedYAML: helmReleaseSecretYAML(t),
	}
}

// stubReader is a minimal client.Reader that serves a single HelmRepository via
// List on a chosen apiVersion. Other versions report "no matches for kind" so
// the provider's version probing can be exercised. It records whether List was
// called.
type stubReader struct {
	servedVersion string
	obj           *unstructured.Unstructured // nil => empty list on the served version
	listCalled    bool
}

func (s *stubReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return fmt.Errorf("Get is not used by the provider")
}

func (s *stubReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	s.listCalled = true
	ul, ok := list.(*unstructured.UnstructuredList)
	if !ok {
		return fmt.Errorf("expected an UnstructuredList")
	}
	gvk := ul.GroupVersionKind()
	if gvk.Version != s.servedVersion {
		return fmt.Errorf("no matches for kind %q in version %q", gvk.Kind, gvk.GroupVersion().String())
	}
	if s.obj != nil {
		ul.Items = []unstructured.Unstructured{*s.obj.DeepCopy()}
	}
	return nil
}

func clusterHelmRelease(version string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "helm.toolkit.fluxcd.io/" + version,
		"kind":       "HelmRelease",
		"metadata": map[string]interface{}{
			"name":            "podinfo",
			"namespace":       "default",
			"resourceVersion": "12345",
			"uid":             "abc-123",
		},
		"spec": map[string]interface{}{
			// A non-default field that must be preserved from the live resource.
			"interval": "1m0s",
			"chart": map[string]interface{}{
				"spec": map[string]interface{}{
					"chart": "podinfo",
					"sourceRef": map[string]interface{}{
						"kind":      "HelmRepository",
						"name":      "podinfo",
						"namespace": "flux-system",
					},
				},
			},
		},
		"status": map[string]interface{}{
			"observedGeneration": int64(1),
		},
	}}
}

func TestFluxHelmReleaseProvider_Handles(t *testing.T) {
	p := FluxHelmReleaseProvider{}
	if !p.Handles(helmReleaseParams(t)) {
		t.Fatal("expected provider to handle a Helm release secret")
	}
	other := helmReleaseParams(t)
	other.InterceptedName = "my-app-config"
	if p.Handles(other) {
		t.Fatal("expected provider not to handle a non-helm secret")
	}
	notSecret := helmReleaseParams(t)
	notSecret.InterceptedGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	if p.Handles(notSecret) {
		t.Fatal("expected provider not to handle a Deployment")
	}
}

func TestFluxHelmReleaseProvider_CopiesExistingFromCluster(t *testing.T) {
	cluster := &stubReader{servedVersion: "v2", obj: clusterHelmRelease("v2")}
	rc := RenderContext{Ctx: fakeClientCtx(t), Params: helmReleaseParams(t), Worktree: newMemWorktree(t), Cluster: cluster}

	out := &ArtifactSet{}
	if err := (FluxHelmReleaseProvider{}).Render(rc, out); err != nil {
		t.Fatalf("render: %v", err)
	}

	if len(out.Items) != 1 {
		t.Fatalf("expected only the HelmRelease artifact, got %d", len(out.Items))
	}
	if !cluster.listCalled {
		t.Error("expected the cluster to be queried for the existing HelmRelease")
	}

	body := string(out.Items[0].Content)
	if !strings.Contains(body, "kind: HelmRelease") {
		t.Errorf("expected a HelmRelease manifest:\n%s", body)
	}
	// The existing chart sourceRef and custom spec are preserved from the cluster.
	if !strings.Contains(body, "name: podinfo") || !strings.Contains(body, "namespace: flux-system") {
		t.Errorf("expected the preserved chart sourceRef:\n%s", body)
	}
	if !strings.Contains(body, "interval: 1m0s") {
		t.Errorf("expected the existing spec.interval to be preserved:\n%s", body)
	}
	// spec.values is overridden with the secret's user-supplied values.
	if !strings.Contains(body, "replicaCount: 2") {
		t.Errorf("expected the secret's values to override spec.values:\n%s", body)
	}
	// The RemoteSyncer's excluded (server-managed) fields are stripped.
	for _, field := range []string{"uid:", "resourceVersion:", "status:", "creationTimestamp:"} {
		if strings.Contains(body, field) {
			t.Errorf("expected the excluded field %q to be removed:\n%s", field, body)
		}
	}
}

func TestFluxHelmReleaseProvider_ClusterVersionProbing(t *testing.T) {
	// The served apiVersion is not known upfront; the provider probes v2, then the
	// beta versions. Here only v2beta2 is served.
	cluster := &stubReader{servedVersion: "v2beta2", obj: clusterHelmRelease("v2beta2")}
	rc := RenderContext{Ctx: fakeClientCtx(t), Params: helmReleaseParams(t), Worktree: newMemWorktree(t), Cluster: cluster}

	out := &ArtifactSet{}
	if err := (FluxHelmReleaseProvider{}).Render(rc, out); err != nil {
		t.Fatalf("render: %v", err)
	}

	if len(out.Items) != 1 {
		t.Fatalf("expected only the synthesized HelmRelease artifact, got %d", len(out.Items))
	}
	if !cluster.listCalled {
		t.Error("expected the cluster to be queried")
	}

	body := string(out.Items[0].Content)
	if !strings.Contains(body, "kind: HelmRelease") {
		t.Errorf("expected a HelmRelease manifest:\n%s", body)
	}
	if !strings.Contains(body, "name: podinfo") || !strings.Contains(body, "namespace: flux-system") {
		t.Errorf("expected the HelmRelease to be copied from the v2beta2 cluster resource:\n%s", body)
	}
}

func TestFluxHelmReleaseProvider_AbsentEverywhere(t *testing.T) {
	wt := newMemWorktree(t) // no HelmRepository in repo

	cluster := &stubReader{servedVersion: "v1", obj: nil} // empty list in cluster
	rc := RenderContext{Ctx: context.Background(), Params: helmReleaseParams(t), Worktree: wt, Cluster: cluster}

	out := &ArtifactSet{}
	if err := (FluxHelmReleaseProvider{}).Render(rc, out); err != nil {
		t.Fatalf("render: %v", err)
	}

	if len(out.Items) != 0 {
		t.Fatalf("expected no artifacts when no HelmRepository can be found, got %d", len(out.Items))
	}
}

func TestFluxHelmReleaseProvider_NilCluster(t *testing.T) {
	wt := newMemWorktree(t) // no HelmRepository in repo
	rc := RenderContext{Ctx: context.Background(), Params: helmReleaseParams(t), Worktree: wt, Cluster: nil}

	out := &ArtifactSet{}
	if err := (FluxHelmReleaseProvider{}).Render(rc, out); err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(out.Items) != 0 {
		t.Fatalf("expected no artifacts with no cluster and no repo, got %d", len(out.Items))
	}
}

func TestFluxHelmReleaseProvider_Deletion(t *testing.T) {
	wt := newMemWorktree(t)
	params := helmReleaseParams(t)
	params.InterceptedYAML = "" // deletion

	rc := RenderContext{Ctx: context.Background(), Params: params, Worktree: wt}
	out := &ArtifactSet{}
	if err := (FluxHelmReleaseProvider{}).Render(rc, out); err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(out.Items) != 1 || !out.Items[0].IsDeletion() {
		t.Fatalf("expected a single deletion artifact, got %+v", out.Items)
	}
	if out.Items[0].GVR != helmReleaseGVR {
		t.Errorf("expected the deletion artifact to carry the HelmRelease GVR, got %+v", out.Items[0].GVR)
	}
}
