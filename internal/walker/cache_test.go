package walker

import (
	"strconv"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
)

// demoSelector matches the demoDeploymentYAML object.
func demoSelector() ObjectSelector {
	return ObjectSelector{GVR: deploymentGVR(), Name: "demo", Namespace: "default"}
}

// deploymentWithReplicas returns a deployment that still matches demoSelector but
// carries a distinct replica count, so a replacement leaves a matchable document
// behind (unlike replacing with arbitrary text).
func deploymentWithReplicas(n int) string {
	return strings.Replace(demoDeploymentYAML, "replicas: 1", "replicas: "+strconv.Itoa(n), 1)
}

func mustRead(t *testing.T, wt *git.Worktree, path string) string {
	t.Helper()
	content, err := readWorktreeFile(wt, path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

const docCacheScope = "https://example.test/repo.git#main"

// enableDocCache turns the document cache on for a test and resets it back to the
// disabled (nil) state afterwards so it never leaks into other tests.
func enableDocCache(t *testing.T) {
	t.Helper()
	InitDocumentCache(16)
	t.Cleanup(func() { InitDocumentCache(0) })
}

func TestReplaceObject_CacheHitSkipsWalk(t *testing.T) {
	enableDocCache(t)
	wt := newMemWorktree(t)
	seedWorktreeFile(t, wt, "a/deploy.yaml", demoDeploymentYAML)
	seedWorktreeFile(t, wt, "b/deploy.yaml", demoDeploymentYAML)

	sel := demoSelector()
	key := docCacheKey{Scope: docCacheScope, Sel: sel}
	docCache.Set(key, docCacheValue{Path: "a/deploy.yaml"})

	claimed, err := ReplaceObject(wt, docCacheScope, sel, []byte(deploymentWithReplicas(2)))
	if err != nil {
		t.Fatalf("ReplaceObject: %v", err)
	}

	// Only the cached file is rewritten; the walk never runs, so the sibling copy
	// is left untouched.
	if got := mustRead(t, wt, "a/deploy.yaml"); !strings.Contains(got, "replicas: 2") {
		t.Errorf("cached file not rewritten:\n%s", got)
	}
	if got := mustRead(t, wt, "b/deploy.yaml"); !strings.Contains(got, "replicas: 1") {
		t.Errorf("non-cached file was rewritten (walk should have been skipped):\n%s", got)
	}
	if len(claimed.Add) != 1 || claimed.Add[0] != "a/deploy.yaml" {
		t.Errorf("claimed = %+v, want Add=[a/deploy.yaml]", claimed)
	}
	if v, ok := docCache.Get(key); !ok || v.Path != "a/deploy.yaml" {
		t.Errorf("cache entry = (%+v, %v), want path a/deploy.yaml", v, ok)
	}
}

func TestReplaceObject_StaleCacheFallsBackToWalk(t *testing.T) {
	enableDocCache(t)
	wt := newMemWorktree(t)
	// The cached path exists but no longer holds the document; the real document
	// lives elsewhere.
	seedWorktreeFile(t, wt, "old/deploy.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: other\n")
	seedWorktreeFile(t, wt, "moved/deploy.yaml", demoDeploymentYAML)

	sel := demoSelector()
	key := docCacheKey{Scope: docCacheScope, Sel: sel}
	docCache.Set(key, docCacheValue{Path: "old/deploy.yaml"})

	if _, err := ReplaceObject(wt, docCacheScope, sel, []byte(deploymentWithReplicas(2))); err != nil {
		t.Fatalf("ReplaceObject: %v", err)
	}

	if got := mustRead(t, wt, "moved/deploy.yaml"); !strings.Contains(got, "replicas: 2") {
		t.Errorf("fallback walk did not rewrite the moved document:\n%s", got)
	}
	if v, ok := docCache.Get(key); !ok || v.Path != "moved/deploy.yaml" {
		t.Errorf("cache entry = (%+v, %v), want re-cached path moved/deploy.yaml", v, ok)
	}
}

func TestReplaceObject_MultiMatchNeverCaches(t *testing.T) {
	enableDocCache(t)
	wt := newMemWorktree(t)
	seedWorktreeFile(t, wt, "a.yaml", demoDeploymentYAML)
	seedWorktreeFile(t, wt, "b.yaml", demoDeploymentYAML)

	sel := demoSelector()
	key := docCacheKey{Scope: docCacheScope, Sel: sel}

	// First run: a full walk matches both files and marks the key never-cache.
	if _, err := ReplaceObject(wt, docCacheScope, sel, []byte(deploymentWithReplicas(2))); err != nil {
		t.Fatalf("ReplaceObject #1: %v", err)
	}
	if v, ok := docCache.Get(key); !ok || !v.NoCache {
		t.Fatalf("cache entry = (%+v, %v), want NoCache=true", v, ok)
	}

	// Second run: never-cache forces another full walk, so both copies are
	// rewritten again.
	if _, err := ReplaceObject(wt, docCacheScope, sel, []byte(deploymentWithReplicas(3))); err != nil {
		t.Fatalf("ReplaceObject #2: %v", err)
	}
	for _, p := range []string{"a.yaml", "b.yaml"} {
		if got := mustRead(t, wt, p); !strings.Contains(got, "replicas: 3") {
			t.Errorf("%s not rewritten on the second run:\n%s", p, got)
		}
	}
}

func TestReplaceObject_SingleMatchCachesPath(t *testing.T) {
	enableDocCache(t)
	wt := newMemWorktree(t)
	seedWorktreeFile(t, wt, "x/deploy.yaml", demoDeploymentYAML)

	sel := demoSelector()
	if _, err := ReplaceObject(wt, docCacheScope, sel, []byte(deploymentWithReplicas(2))); err != nil {
		t.Fatalf("ReplaceObject: %v", err)
	}
	v, ok := docCache.Get(docCacheKey{Scope: docCacheScope, Sel: sel})
	if !ok || v.NoCache || v.Path != "x/deploy.yaml" {
		t.Errorf("cache entry = (%+v, %v), want path x/deploy.yaml", v, ok)
	}
}

func TestReplaceObject_DeletionClearsKey(t *testing.T) {
	enableDocCache(t)
	wt := newMemWorktree(t)
	seedWorktreeFile(t, wt, "x/deploy.yaml", demoDeploymentYAML)

	sel := demoSelector()
	key := docCacheKey{Scope: docCacheScope, Sel: sel}
	docCache.Set(key, docCacheValue{Path: "x/deploy.yaml"})

	claimed, err := ReplaceObject(wt, docCacheScope, sel, nil)
	if err != nil {
		t.Fatalf("ReplaceObject: %v", err)
	}
	if len(claimed.Delete) != 1 {
		t.Fatalf("claimed = %+v, want one deleted path", claimed)
	}
	if _, ok := docCache.Get(key); ok {
		t.Error("cache entry survived a deletion")
	}
	if _, err := wt.Filesystem.Stat("x/deploy.yaml"); err == nil {
		t.Error("expected the now-empty file to be removed")
	}
}

func TestReplaceObject_DisabledCacheRewritesAllMatches(t *testing.T) {
	// No InitDocumentCache call: docCache stays nil (caching disabled).
	if docCache != nil {
		t.Fatalf("docCache should be nil by default")
	}
	wt := newMemWorktree(t)
	seedWorktreeFile(t, wt, "a.yaml", demoDeploymentYAML)
	seedWorktreeFile(t, wt, "b.yaml", demoDeploymentYAML)

	sel := demoSelector()
	if _, err := ReplaceObject(wt, docCacheScope, sel, []byte(deploymentWithReplicas(2))); err != nil {
		t.Fatalf("ReplaceObject: %v", err)
	}
	for _, p := range []string{"a.yaml", "b.yaml"} {
		if got := mustRead(t, wt, p); !strings.Contains(got, "replicas: 2") {
			t.Errorf("%s not rewritten with caching disabled:\n%s", p, got)
		}
	}
}
