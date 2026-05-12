package mutator

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newDeploymentFinder(name, namespace, replacement string) resourceFinderImplem { // nolint:unparam
	return resourceFinderImplem{
		searchedGVK: schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
		searchedName:      name,
		searchedNamespace: namespace,
		content:           []byte(replacement),
	}
}

func TestResourceFinder_replaceResourceIfFound(t *testing.T) {
	t.Run("no match returns input unchanged", func(t *testing.T) {
		rf := newDeploymentFinder("demo", "default", "REPLACED")
		in := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: other
  namespace: default
spec:
  replicas: 1
`)
		got := rf.replaceResourceIfFound(in)
		if string(got) != string(in) {
			t.Errorf("expected unchanged; got:\n%s", got)
		}
	})

	t.Run("single doc match is replaced by rf.content", func(t *testing.T) {
		rf := newDeploymentFinder("demo", "default", "REPLACED")
		in := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
spec:
  replicas: 1
`)
		got := string(rf.replaceResourceIfFound(in))
		if !strings.Contains(got, "REPLACED") {
			t.Errorf("expected replacement content in output, got:\n%s", got)
		}
		if strings.Contains(got, "replicas: 1") {
			t.Errorf("original content should have been replaced, got:\n%s", got)
		}
	})

	t.Run("multi-doc only the matching doc is replaced", func(t *testing.T) {
		rf := newDeploymentFinder("demo", "default", "REPLACED")
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
		got := string(rf.replaceResourceIfFound(in))
		if !strings.Contains(got, "keep-me") {
			t.Errorf("non-matching doc should be preserved, got:\n%s", got)
		}
		if !strings.Contains(got, "REPLACED") {
			t.Errorf("matching doc should be replaced, got:\n%s", got)
		}
		if strings.Contains(got, "replicas: 1") {
			t.Errorf("replaced doc should no longer contain original body, got:\n%s", got)
		}
	})

	t.Run("empty searched namespace matches any namespace", func(t *testing.T) {
		rf := newDeploymentFinder("demo", "", "REPLACED")
		in := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: any-namespace
spec:
  replicas: 1
`)
		got := string(rf.replaceResourceIfFound(in))
		if !strings.Contains(got, "REPLACED") {
			t.Errorf("expected replacement when searched namespace is empty, got:\n%s", got)
		}
	})

	t.Run("unparseable doc between valid docs is preserved", func(t *testing.T) {
		rf := newDeploymentFinder("demo", "default", "REPLACED")
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
		got := string(rf.replaceResourceIfFound(in))
		if !strings.Contains(got, "REPLACED") {
			t.Errorf("matching doc should be replaced, got:\n%s", got)
		}
		if !strings.Contains(got, "this is : not : valid") {
			t.Errorf("unparseable doc should be preserved verbatim, got:\n%s", got)
		}
		if !strings.Contains(got, "keeper") {
			t.Errorf("third doc should be preserved, got:\n%s", got)
		}
	})

	t.Run("helm values without resource-finder comment are preserved", func(t *testing.T) {
		rf := newDeploymentFinder("demo", "default", "REPLACED")
		in := []byte(`replicaCount: 3
image:
  repository: nginx
  tag: latest
service:
  type: ClusterIP
  port: 80
`)
		got := rf.replaceResourceIfFound(in)
		if string(got) != string(in) {
			t.Errorf("expected helm values without comment to be unchanged; got:\n%s", got)
		}
	})

	t.Run("helm values with resource-finder comment matching is replaced", func(t *testing.T) {
		rf := newDeploymentFinder("demo", "default", "REPLACED")
		in := []byte(ResourceFinderCommentPrefix + `default/demo
replicaCount: 3
image:
  repository: nginx
  tag: latest
`)
		got := string(rf.replaceResourceIfFound(in))
		if !strings.Contains(got, "REPLACED") {
			t.Errorf("expected matching helm values doc to be replaced, got:\n%s", got)
		}
		if strings.Contains(got, "replicaCount: 3") {
			t.Errorf("original helm values should have been replaced, got:\n%s", got)
		}
	})

	t.Run("helm values with resource-finder comment not matching is preserved", func(t *testing.T) {
		rf := newDeploymentFinder("demo", "default", "REPLACED")
		in := []byte(ResourceFinderCommentPrefix + `default/other
replicaCount: 3
image:
  repository: nginx
`)
		got := rf.replaceResourceIfFound(in)
		if string(got) != string(in) {
			t.Errorf("expected non-matching helm values to be unchanged; got:\n%s", got)
		}
	})

	t.Run("core resource without group uses bare version as apiVersion", func(t *testing.T) {
		rf := resourceFinderImplem{
			searchedGVK: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "configmaps",
			},
			searchedName:      "demo",
			searchedNamespace: "default",
			content:           []byte("REPLACED"),
		}
		in := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: demo
  namespace: default
data:
  foo: bar
`)
		got := string(rf.replaceResourceIfFound(in))
		if !strings.Contains(got, "REPLACED") {
			t.Errorf("expected core resource to be matched and replaced, got:\n%s", got)
		}
	})
}
