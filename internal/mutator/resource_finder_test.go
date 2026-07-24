package mutator

import (
	"io"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/syngit-org/syngit/internal/walker"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/interceptor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// readWorktree reads a worktree-relative file back for assertions.
func readWorktree(t *testing.T, wt *git.Worktree, path string) string {
	t.Helper()
	f, err := wt.Filesystem.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close() // nolint:errcheck
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestResourceFinderPlace_OverridesExistingHelmRelease reproduces the reported
// bug: an existing HelmRelease file living at an arbitrary repo path is updated
// in place, keyed on the release's own identity — not the intercepted Helm secret
// name — so no file is created at the default structured path.
func TestResourceFinderPlace_OverridesExistingHelmRelease(t *testing.T) {
	const existingPath = "clusters/prod/demo.yaml"
	wt := newMemWorktree(t)

	seeded := []byte(`apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: demo
  namespace: prod
spec:
  interval: 1h
  values:
    greeting: old
`)
	if err := walker.WriteWorktreeFile(wt, existingPath, seeded); err != nil {
		t.Fatalf("seed %s: %v", existingPath, err)
	}

	updated := []byte(`apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: demo
  namespace: prod
spec:
  interval: 1h
  values:
    greeting: new
`)
	artifacts := ArtifactSet{Items: []Artifact{{
		GVR:       helmReleaseGVR,
		Name:      "demo",
		Namespace: "prod",
		Content:   updated,
	}}}

	params := interceptor.GitPipelineParams{
		RemoteSyncer: syngit.RemoteSyncer{ObjectMeta: metav1.ObjectMeta{Namespace: "prod"}},
		RemoteTarget: syngit.RemoteTarget{Spec: syngit.RemoteTargetSpec{
			TargetRepository: "https://example.com/repo.git",
			UpstreamBranch:   "main",
		}},
		// The intercepted secret name must NOT be what the finder searches by.
		InterceptedName: "sh.helm.release.v1.demo.v1",
	}

	claimed, err := (ResourceFinder{}).place(params, artifacts, wt)
	if err != nil {
		t.Fatalf("place: %v", err)
	}

	if len(claimed.Add) != 1 || claimed.Add[0] != existingPath {
		t.Fatalf("expected the existing file %q to be claimed, got add=%v delete=%v",
			existingPath, claimed.Add, claimed.Delete)
	}

	got := readWorktree(t, wt, existingPath)
	if !strings.Contains(got, "greeting: new") {
		t.Errorf("expected the existing HelmRelease to be updated in place:\n%s", got)
	}
	if strings.Contains(got, "greeting: old") {
		t.Errorf("expected the old values to be replaced:\n%s", got)
	}
}

// TestResourceFinderPlace_NoMatchClaimsNothing confirms that when no file in the
// repo matches, place claims nothing, so the caller falls back to the default
// structured placement.
func TestResourceFinderPlace_NoMatchClaimsNothing(t *testing.T) {
	wt := newMemWorktree(t) // empty repo

	artifacts := ArtifactSet{Items: []Artifact{{
		GVR:       helmReleaseGVR,
		Name:      "demo",
		Namespace: "prod",
		Content:   []byte("apiVersion: helm.toolkit.fluxcd.io/v2\nkind: HelmRelease\nmetadata:\n  name: demo\n  namespace: prod\n"),
	}}}
	params := interceptor.GitPipelineParams{
		RemoteSyncer: syngit.RemoteSyncer{ObjectMeta: metav1.ObjectMeta{Namespace: "prod"}},
		RemoteTarget: syngit.RemoteTarget{Spec: syngit.RemoteTargetSpec{
			TargetRepository: "https://example.com/repo.git",
			UpstreamBranch:   "main",
		}},
		InterceptedName: "sh.helm.release.v1.demo.v1",
	}

	claimed, err := (ResourceFinder{}).place(params, artifacts, wt)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if claimed.ClaimExists() {
		t.Fatalf("expected no claim on an empty repo, got add=%v delete=%v", claimed.Add, claimed.Delete)
	}
}

// deploymentSelector mirrors how ResourceFinder.place builds its selector: a
// Kubernetes identity plus the resource-finder comment marker.
func deploymentSelector(name, namespace string) walker.ObjectSelector { // nolint:unparam
	return walker.ObjectSelector{
		GVR: schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
		Name:          name,
		Namespace:     namespace,
		CommentPrefix: ResourceFinderCommentPrefix,
	}
}

func TestReplaceDocInContent(t *testing.T) { // nolint:gocyclo
	const replacement = "REPLACED"

	t.Run("no match returns input unchanged", func(t *testing.T) {
		in := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: other
  namespace: default
spec:
  replicas: 1
`)
		got, found := walker.ReplaceDocInContent(in, deploymentSelector("demo", "default"), []byte(replacement))
		if found {
			t.Error("expected no match")
		}
		if string(got) != string(in) {
			t.Errorf("expected unchanged; got:\n%s", got)
		}
	})

	t.Run("single doc match is replaced", func(t *testing.T) {
		in := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
spec:
  replicas: 1
`)
		got, found := walker.ReplaceDocInContent(in, deploymentSelector("demo", "default"), []byte(replacement))
		if !found {
			t.Fatal("expected a match")
		}
		stringGot := string(got)
		if !strings.Contains(stringGot, replacement) {
			t.Errorf("expected replacement content in output, got:\n%s", got)
		}
		if strings.Contains(stringGot, "replicas: 1") {
			t.Errorf("original content should have been replaced, got:\n%s", got)
		}
	})

	t.Run("multi-doc only the matching doc is replaced", func(t *testing.T) {
		in := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: keep-me
  namespace: default
data:
  foo: bar
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
spec:
  replicas: 1
`)
		got, found := walker.ReplaceDocInContent(in, deploymentSelector("demo", "default"), []byte(replacement))
		if !found {
			t.Fatal("expected a match")
		}
		stringGot := string(got)
		if !strings.Contains(stringGot, "keep-me") {
			t.Errorf("non-matching doc should be preserved, got:\n%s", got)
		}
		if !strings.Contains(stringGot, replacement) {
			t.Errorf("matching doc should be replaced, got:\n%s", got)
		}
		if strings.Contains(stringGot, "replicas: 1") {
			t.Errorf("replaced doc should no longer contain original body, got:\n%s", got)
		}
	})

	t.Run("empty searched namespace matches the default namespace", func(t *testing.T) {
		in := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
spec:
  replicas: 1
`)
		got, found := walker.ReplaceDocInContent(in, deploymentSelector("demo", ""), []byte(replacement))
		if !found {
			t.Fatal("expected a match against the default namespace")
		}
		if !strings.Contains(string(got), replacement) {
			t.Errorf("expected replacement when searched namespace is empty, got:\n%s", got)
		}

		// A non-default namespace is not matched by an empty searched namespace.
		other := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: any-namespace
spec:
  replicas: 1
`)
		if _, found := walker.ReplaceDocInContent(other, deploymentSelector("demo", ""), []byte(replacement)); found {
			t.Error("expected no match for a non-default namespace when searched namespace is empty")
		}
	})

	t.Run("version-agnostic match across apiVersions", func(t *testing.T) {
		// The selector targets apps/v1 but the manifest is apps/v1beta1; matching
		// is version-agnostic so it is still replaced.
		in := []byte(`apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: demo
  namespace: default
spec:
  replicas: 1
`)
		got, found := walker.ReplaceDocInContent(in, deploymentSelector("demo", "default"), []byte(replacement))
		if !found {
			t.Fatal("expected a version-agnostic match")
		}
		if !strings.Contains(string(got), replacement) {
			t.Errorf("expected replacement, got:\n%s", got)
		}
	})

	t.Run("unparseable doc between valid docs is preserved", func(t *testing.T) {
		in := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
---
this is : not : valid : yaml : [
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: keeper
  namespace: default
`)
		got, found := walker.ReplaceDocInContent(in, deploymentSelector("demo", "default"), []byte(replacement))
		if !found {
			t.Fatal("expected a match")
		}
		stringGot := string(got)
		if !strings.Contains(stringGot, replacement) {
			t.Errorf("matching doc should be replaced, got:\n%s", got)
		}
		if !strings.Contains(stringGot, "this is : not : valid") {
			t.Errorf("unparseable doc should be preserved verbatim, got:\n%s", got)
		}
		if !strings.Contains(stringGot, "keeper") {
			t.Errorf("third doc should be preserved, got:\n%s", got)
		}
	})

	t.Run("helm values without resource-finder comment are preserved", func(t *testing.T) {
		in := []byte(`replicaCount: 3
image:
  repository: nginx
  tag: latest
service:
  type: ClusterIP
  port: 80
`)
		got, found := walker.ReplaceDocInContent(in, deploymentSelector("demo", "default"), []byte(replacement))
		if found {
			t.Error("expected no match")
		}
		if string(got) != string(in) {
			t.Errorf("expected helm values without comment to be unchanged; got:\n%s", got)
		}
	})

	t.Run("helm values with resource-finder comment matching is replaced", func(t *testing.T) {
		in := []byte(ResourceFinderCommentPrefix + `default/demo
replicaCount: 3
image:
  repository: nginx
  tag: latest
`)
		got, found := walker.ReplaceDocInContent(in, deploymentSelector("demo", "default"), []byte(replacement))
		if !found {
			t.Fatal("expected a comment-marker match")
		}
		stringGot := string(got)
		if !strings.Contains(stringGot, replacement) {
			t.Errorf("expected matching helm values doc to be replaced, got:\n%s", got)
		}
		if strings.Contains(stringGot, "replicaCount: 3") {
			t.Errorf("original helm values should have been replaced, got:\n%s", got)
		}
	})

	t.Run("helm values with resource-finder comment not matching is preserved", func(t *testing.T) {
		in := []byte(ResourceFinderCommentPrefix + `default/other
replicaCount: 3
image:
  repository: nginx
`)
		got, found := walker.ReplaceDocInContent(in, deploymentSelector("demo", "default"), []byte(replacement))
		if found {
			t.Error("expected no match")
		}
		if string(got) != string(in) {
			t.Errorf("expected non-matching helm values to be unchanged; got:\n%s", got)
		}
	})

	t.Run("core resource without group uses bare version as apiVersion", func(t *testing.T) {
		sel := walker.ObjectSelector{
			GVR: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "configmaps",
			},
			Name:          "demo",
			Namespace:     "default",
			CommentPrefix: ResourceFinderCommentPrefix,
		}
		in := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: demo
  namespace: default
data:
  foo: bar
`)
		got, found := walker.ReplaceDocInContent(in, sel, []byte(replacement))
		if !found {
			t.Fatal("expected a match")
		}
		if !strings.Contains(string(got), replacement) {
			t.Errorf("expected core resource to be matched and replaced, got:\n%s", got)
		}
	})

	t.Run("empty content deletes the matching doc", func(t *testing.T) {
		in := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: keep-me
  namespace: default
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
`)
		got, found := walker.ReplaceDocInContent(in, deploymentSelector("demo", "default"), nil)
		if !found {
			t.Fatal("expected a match")
		}
		stringGot := string(got)
		if strings.Contains(stringGot, "name: demo") {
			t.Errorf("deleted doc should be gone, got:\n%s", got)
		}
		if !strings.Contains(stringGot, "keep-me") {
			t.Errorf("sibling doc should be preserved, got:\n%s", got)
		}
	})
}
