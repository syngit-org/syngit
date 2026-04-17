package envtest

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

// ObjectFile describes a file in a repository that contains a matching K8s
// object manifest.
type ObjectFile struct {
	Path    string
	Content []byte
}

// IsObjectInRepo reports whether any YAML file on branch contains a K8s
// object whose apiVersion, kind, name, and namespace match obj.
func (gs *GitServer) IsObjectInRepo(repo RepoRef, branch string, obj runtime.Object) (bool, error) {
	files, err := gs.searchForObject(repo, branch, obj, false)
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

// SearchForObjectInRepo returns all files on branch whose YAML content
// matches obj's identity and whose spec/data match obj's spec/data.
func (gs *GitServer) SearchForObjectInRepo(repo RepoRef, branch string, obj runtime.Object) ([]ObjectFile, error) {
	return gs.searchForObject(repo, branch, obj, true)
}

// IsFieldDefined reports whether yamlPath (dot-separated) is defined in the
// YAML file that contains obj on branch.
func (gs *GitServer) IsFieldDefined(repo RepoRef, branch string, obj runtime.Object, yamlPath string) (bool, error) {
	files, err := gs.searchForObject(repo, branch, obj, false)
	if err != nil {
		return false, err
	}
	for _, f := range files {
		var parsed map[string]any
		if err := yaml.Unmarshal(f.Content, &parsed); err != nil {
			continue
		}
		if _, ok := lookupYamlField(parsed, yamlPath); ok {
			return true, nil
		}
	}
	return false, nil
}

func (gs *GitServer) searchForObject(
	repo RepoRef,
	branch string,
	obj runtime.Object,
	matchSpec bool,
) ([]ObjectFile, error) {
	paths, err := gs.ListFiles(repo, branch)
	if err != nil {
		return nil, err
	}
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("access metadata: %w", err)
	}

	var objSpec map[string]any
	if matchSpec {
		objYaml, err := yaml.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("marshal object: %w", err)
		}
		if err := yaml.Unmarshal(objYaml, &objSpec); err != nil {
			return nil, fmt.Errorf("unmarshal object: %w", err)
		}
	}

	matches := []ObjectFile{}
	for _, p := range paths {
		content, err := gs.ReadFile(repo, branch, p)
		if err != nil {
			continue
		}
		if !documentMatchesIdentity(content, obj, metadata) {
			continue
		}
		if matchSpec && !documentMatchesSpec(content, objSpec) {
			continue
		}
		matches = append(matches, ObjectFile{Path: p, Content: content})
	}
	return matches, nil
}

// documentMatchesIdentity returns true if any YAML document in content has
// apiVersion/kind/name/namespace matching obj.
func documentMatchesIdentity(content []byte, obj runtime.Object, metadata metav1.Object) bool {
	gvk := obj.GetObjectKind().GroupVersionKind()
	wantAPIVersion := gvk.GroupVersion().String()
	wantKind := gvk.Kind
	wantName := metadata.GetName()
	wantNamespace := metadata.GetNamespace()

	for _, doc := range splitYamlDocuments(content) {
		var parsed map[string]any
		if err := yaml.Unmarshal(doc, &parsed); err != nil {
			continue
		}
		apiVersion, _ := lookupYamlField(parsed, "apiVersion")
		kind, _ := lookupYamlField(parsed, "kind")
		name, _ := lookupYamlField(parsed, "metadata.name")
		namespace, _ := lookupYamlField(parsed, "metadata.namespace")

		if asString(kind) != wantKind {
			continue
		}
		if asString(name) != wantName {
			continue
		}
		if asString(namespace) != wantNamespace {
			continue
		}
		// Accept api version mismatch when obj has no group/version set
		// (common when passing a typed object without schema info).
		gotAPI := asString(apiVersion)
		if wantAPIVersion != "" && !strings.HasPrefix(wantAPIVersion, "/") && gotAPI != wantAPIVersion {
			continue
		}
		return true
	}
	return false
}

// documentMatchesSpec returns true if any YAML document in content has a
// spec or data field equal to the corresponding field on objSpec.
func documentMatchesSpec(content []byte, objSpec map[string]any) bool {
	for _, doc := range splitYamlDocuments(content) {
		var parsed map[string]any
		if err := yaml.Unmarshal(doc, &parsed); err != nil {
			continue
		}
		if fieldsMatch(parsed, objSpec, "spec") && fieldsMatch(parsed, objSpec, "data") {
			_, hasSpec := parsed["spec"]
			_, hasData := parsed["data"]
			if hasSpec || hasData {
				return true
			}
		}
	}
	return false
}

// fieldsMatch returns true if the given field is either absent from both
// maps or equal (via JSON comparison) in both maps.
func fieldsMatch(a, b map[string]any, field string) bool {
	va, okA := a[field]
	vb, okB := b[field]
	if okA != okB {
		return false
	}
	if !okA {
		return true
	}
	ja, err := json.Marshal(va)
	if err != nil {
		return false
	}
	jb, err := json.Marshal(vb)
	if err != nil {
		return false
	}
	return string(ja) == string(jb)
}

// splitYamlDocuments splits multi-document YAML content by the "---"
// separator, dropping empty documents.
func splitYamlDocuments(content []byte) [][]byte {
	parts := strings.Split(string(content), "\n---\n")
	docs := [][]byte{}
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" || trimmed == "---" {
			continue
		}
		docs = append(docs, []byte(trimmed))
	}
	if len(docs) == 0 {
		return [][]byte{content}
	}
	return docs
}

// lookupYamlField walks a dot-separated path through nested maps and
// returns the value (if any).
func lookupYamlField(parsed map[string]any, path string) (any, bool) {
	keys := strings.Split(path, ".")
	current := any(parsed)
	for _, k := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[k]
		if !ok {
			return nil, false
		}
		current = v
	}
	return current, true
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
