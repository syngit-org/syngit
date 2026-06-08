package walker

import (
	"bytes"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// ObjectSelector identifies a target YAML document inside the worktree, either by
// Kubernetes identity (GVR + name + namespace, version-agnostic) or, when
// CommentPrefix is set, by a Syngit comment marker carried on the document's first
// line.
type ObjectSelector struct {
	GVR       schema.GroupVersionResource
	Name      string
	Namespace string // "" matches the "default" namespace
	// CommentPrefix, when non-empty (e.g. ResourceFinderCommentPrefix), also
	// matches non-Kubernetes documents whose first comment line is
	// "<CommentPrefix><namespace>/<name>".
	CommentPrefix string
}

// matchDoc reports whether a single YAML document satisfies sel. Kubernetes
// documents are matched by group + resource (kind converted via
// UnsafeGuessKindToResource) + name, with the namespace optional and the
// apiVersion version ignored. Non-Kubernetes documents are matched, when
// sel.CommentPrefix is set, by their first-line comment marker. Unparseable
// documents never match (they are preserved verbatim by the rewriters).
func matchDoc(doc []byte, sel ObjectSelector) bool {
	if matchComment(doc, sel) {
		return true
	}

	m := map[string]interface{}{}
	if err := yaml.Unmarshal(doc, &m); err != nil || len(m) == 0 {
		return false
	}

	apiVersion, _ := m["apiVersion"].(string)
	kind, _ := m["kind"].(string)
	if apiVersion == "" && kind == "" {
		return false
	}

	group := ""
	if g, _, ok := strings.Cut(apiVersion, "/"); ok {
		group = g
	}
	name, namespace := "", ""
	if md, ok := m["metadata"].(map[string]interface{}); ok {
		name, _ = md["name"].(string)
		namespace, _ = md["namespace"].(string)
	}

	return group == sel.GVR.Group &&
		kindToResource(kind) == sel.GVR.Resource &&
		name == sel.Name &&
		((sel.Namespace == "" && namespace == "default") || (namespace == sel.Namespace))
}

// matchComment reports whether doc's first line carries the Syngit marker
// "<sel.CommentPrefix><namespace>/<name>" matching sel. It returns false when
// sel.CommentPrefix is empty.
func matchComment(doc []byte, sel ObjectSelector) bool {
	if sel.CommentPrefix == "" {
		return false
	}
	firstLine, _, _ := bytes.Cut(doc, []byte("\n"))
	markerLine := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(firstLine), []byte("#")))
	prefix := []byte(sel.CommentPrefix)
	if !bytes.HasPrefix(markerLine, prefix) {
		return false
	}
	value := string(bytes.TrimPrefix(markerLine, prefix))
	ns, name, hasSep := strings.Cut(value, "/")
	if !hasSep {
		name = ns
		ns = ""
	}
	return name == sel.Name && ns == sel.Namespace
}

// SelectorFromDoc derives a selector from a document's own Kubernetes identity so
// a write can locate "the document that equals me" within an existing file.
func SelectorFromDoc(doc []byte) ObjectSelector {
	m := map[string]interface{}{}
	_ = yaml.Unmarshal(doc, &m)

	apiVersion, _ := m["apiVersion"].(string)
	group := ""
	if g, _, ok := strings.Cut(apiVersion, "/"); ok {
		group = g
	}
	kind, _ := m["kind"].(string)
	name, namespace := "", ""
	if md, ok := m["metadata"].(map[string]interface{}); ok {
		name, _ = md["name"].(string)
		namespace, _ = md["namespace"].(string)
	}

	return ObjectSelector{
		GVR:       schema.GroupVersionResource{Group: group, Resource: kindToResource(kind)},
		Name:      name,
		Namespace: namespace,
	}
}

// kindToResource converts a Kubernetes Kind (singular) to its resource (plural)
// name; the empty kind maps to the empty resource.
func kindToResource(kind string) string {
	if kind == "" {
		return ""
	}
	gvr, _ := meta.UnsafeGuessKindToResource(schema.GroupVersionKind{Kind: kind})
	return gvr.Resource
}
