package walker

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func deploymentGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
}

const demoDeploymentYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
spec:
  replicas: 1
`

func TestFindObject(t *testing.T) {
	wt := newMemWorktree(t)
	seedWorktreeFile(t, wt, "apps/deploy.yaml", demoDeploymentYAML)

	sel := ObjectSelector{GVR: deploymentGVR(), Name: "demo", Namespace: "default"}
	path, doc, found, err := FindObject(wt, sel)
	if err != nil {
		t.Fatalf("FindObject: %v", err)
	}
	if !found {
		t.Fatal("expected to find the deployment")
	}
	if path != "apps/deploy.yaml" {
		t.Errorf("path = %q, want apps/deploy.yaml", path)
	}
	if !strings.Contains(string(doc), "name: demo") {
		t.Errorf("returned doc does not hold the object:\n%s", doc)
	}
}

func TestFindObject_NotFound(t *testing.T) {
	wt := newMemWorktree(t)
	seedWorktreeFile(t, wt, "cm.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: other\n")

	sel := ObjectSelector{GVR: schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, Name: "demo"}
	_, _, found, err := FindObject(wt, sel)
	if err != nil {
		t.Fatalf("FindObject: %v", err)
	}
	if found {
		t.Fatal("did not expect a match")
	}
}

func TestReplaceObject(t *testing.T) {
	wt := newMemWorktree(t)
	seedWorktreeFile(t, wt, "deploy.yaml", demoDeploymentYAML)

	sel := ObjectSelector{GVR: deploymentGVR(), Name: "demo", Namespace: "default"}
	claimed, err := ReplaceObject(wt, "", sel, []byte("REPLACED\n"))
	if err != nil {
		t.Fatalf("ReplaceObject: %v", err)
	}
	if !claimed.ClaimExists() {
		t.Fatal("expected a claimed path")
	}

	got, err := readWorktreeFile(wt, "deploy.yaml")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(got), "REPLACED") || strings.Contains(string(got), "replicas: 1") {
		t.Errorf("file not replaced:\n%s", got)
	}
}

func TestReplaceObject_Deletion(t *testing.T) {
	wt := newMemWorktree(t)
	seedWorktreeFile(t, wt, "deploy.yaml", demoDeploymentYAML)

	sel := ObjectSelector{GVR: deploymentGVR(), Name: "demo", Namespace: "default"}
	claimed, err := ReplaceObject(wt, "", sel, nil)
	if err != nil {
		t.Fatalf("ReplaceObject: %v", err)
	}
	if len(claimed.Delete) != 1 {
		t.Fatalf("expected one deleted path, got %+v", claimed)
	}
	if _, err := wt.Filesystem.Stat("deploy.yaml"); err == nil {
		t.Error("expected the now-empty file to be removed")
	}
}

func TestWriteObjectAtPath_MergesExistingFile(t *testing.T) {
	wt := newMemWorktree(t)
	seedWorktreeFile(t, wt, "multi.yaml", `apiVersion: v1
kind: ConfigMap
metadata:
  name: keep
  namespace: default
data:
  a: b
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
spec:
  replicas: 1
`)
	newDoc := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
spec:
  replicas: 5
`)
	claimed, err := WriteObjectAtPath(wt, "multi.yaml", SelectorFromDoc(newDoc), newDoc)
	if err != nil {
		t.Fatalf("WriteObjectAtPath: %v", err)
	}
	if !claimed.ClaimExists() {
		t.Fatal("expected a claimed path")
	}

	got, _ := readWorktreeFile(wt, "multi.yaml")
	s := string(got)
	if !strings.Contains(s, "name: keep") {
		t.Errorf("sibling doc lost:\n%s", s)
	}
	if !strings.Contains(s, "replicas: 5") || strings.Contains(s, "replicas: 1") {
		t.Errorf("matched doc not replaced:\n%s", s)
	}
}

func TestWriteObjectAtPath_NewFileAndDeletion(t *testing.T) {
	wt := newMemWorktree(t)

	doc := []byte(demoDeploymentYAML)
	if _, err := WriteObjectAtPath(wt, "nested/dir/deploy.yaml", SelectorFromDoc(doc), doc); err != nil {
		t.Fatalf("WriteObjectAtPath create: %v", err)
	}
	if _, err := wt.Filesystem.Stat("nested/dir/deploy.yaml"); err != nil {
		t.Fatalf("expected the file (and parents) to be created: %v", err)
	}

	claimed, err := WriteObjectAtPath(wt, "nested/dir/deploy.yaml", ObjectSelector{}, nil)
	if err != nil {
		t.Fatalf("WriteObjectAtPath delete: %v", err)
	}
	if len(claimed.Delete) != 1 {
		t.Fatalf("expected a deletion claim, got %+v", claimed)
	}
	if _, err := wt.Filesystem.Stat("nested/dir/deploy.yaml"); err == nil {
		t.Error("expected file to be removed")
	}
}
