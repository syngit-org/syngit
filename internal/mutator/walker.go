package mutator

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// walker.go is the single home for every worktree manipulation in this package.
// It exposes three capabilities on top of one recursive file walk:
//   - search a Kubernetes object or a Syngit comment marker (FindObject),
//   - dynamically replace a matching document across the worktree (ReplaceObject),
//   - write a document at an explicit path, merging into an existing file
//     (WriteObjectAtPath).
//
// The orchestration types (ResourceFinder, DefaultWorktreeCustomizer) and
// writeArtifactAtPath are thin callers of the functions defined here.

// docSeparator is the canonical YAML document separator used everywhere to split
// and rejoin multi-document worktree files.
var docSeparator = []byte("\n---\n")

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

// -- Public API: search ------------------------------------------------------

// FindObject walks the worktree and returns the first YAML document matching sel,
// together with the worktree-relative path of the file that holds it. found is
// false when nothing matches.
func FindObject(wt *git.Worktree, sel ObjectSelector) (path string, doc []byte, found bool, err error) {
	root := wt.Filesystem.Root()
	werr := WalkWorktreeYAML(wt, root, func(p string, rawDoc []byte) (bool, bool, error) {
		if !matchDoc(rawDoc, sel) {
			return false, true, nil // skip non-matching documents
		}
		path = worktreeRelativePath(root, p)
		doc = append([]byte(nil), rawDoc...)
		found = true
		return true, false, nil // stop at the first match
	})
	return path, doc, found, werr
}

// -- Public API: replace -----------------------------------------------------

// ReplaceObject walks the worktree, replaces the first document matching sel with
// content (deletes it when content is empty), writes the file back preserving
// sibling documents, and returns the claimed paths. It claims nothing when no
// document matches.
func ReplaceObject(wt *git.Worktree, sel ObjectSelector, content []byte) (interceptor.ClaimedPaths, error) {
	claimed := interceptor.NewClaimedPaths()
	root := wt.Filesystem.Root()

	_, err := walkWorktreeFiles(wt, root, func(path string, fileContent []byte) (bool, error) {
		out, found := replaceDocInContent(fileContent, sel, content)
		if !found {
			return false, nil
		}

		if string(out) != string(fileContent) {
			if len(bytes.TrimSpace(out)) == 0 {
				if rerr := removeWorktreeFile(wt, path); rerr != nil {
					return true, fmt.Errorf("failed to remove %s: %w", path, rerr)
				}
			} else if werr := writeWorktreeFile(wt, path, out); werr != nil {
				return true, fmt.Errorf("failed to write %s: %w", path, werr)
			}
		}

		rel := worktreeRelativePath(root, path)
		if len(content) == 0 {
			claimed.AppendDeletedPath(rel)
		} else {
			claimed.AppendAddedPath(rel)
		}
		return false, nil // keep walking: a later file may match too
	})
	if err != nil {
		return interceptor.NewClaimedPaths(), err
	}
	return claimed, nil
}

// WriteObjectAtPath writes content to an explicit worktree path. When the file
// already exists, the document matching sel is replaced in place (or appended
// when none matches) so sibling documents survive; otherwise a new file is
// created. The file is deleted when content is empty. It returns the claimed path.
func WriteObjectAtPath(wt *git.Worktree, path string, sel ObjectSelector, content []byte) (interceptor.ClaimedPaths, error) {
	claimed := interceptor.NewClaimedPaths()
	cleanPath := filepath.Clean(path)

	if len(content) == 0 {
		_ = removeWorktreeFile(wt, cleanPath)
		claimed.AppendDeletedPath(cleanPath)
		return claimed, nil
	}

	out := content
	if existing, err := readWorktreeFile(wt, cleanPath); err == nil {
		if merged, found := replaceDocInContent(existing, sel, content); found {
			out = merged
		} else {
			out = appendDoc(existing, content)
		}
	}

	if err := writeWorktreeFile(wt, cleanPath, out); err != nil {
		return interceptor.NewClaimedPaths(), err
	}
	claimed.AppendAddedPath(cleanPath)
	return claimed, nil
}

// -- Walk primitives ---------------------------------------------------------

// WalkWorktreeYAML recursively visits every .yaml/.yml file under basePath and
// calls visit once per YAML document with the file path and the document bytes.
// visit returns (stop, skip, err): skip moves to the next document, stop ends the
// walk, and a non-nil err aborts it. It is the per-document layer over
// walkWorktreeFiles, kept public for callers that need a custom visitor.
func WalkWorktreeYAML(worktree *git.Worktree, basePath string, visit func(path string, content []byte) (stop bool, skip bool, err error)) error {
	_, err := walkWorktreeFiles(worktree, basePath, func(path string, content []byte) (bool, error) {
		for _, rawDoc := range bytes.Split(content, docSeparator) {
			stop, skip, verr := visit(path, rawDoc)
			if verr != nil {
				return true, verr
			}
			if skip {
				continue
			}
			if stop {
				return true, nil
			}
		}
		return false, nil
	})
	return err
}

// walkWorktreeFiles is the single recursion over the worktree. It visits every
// .yaml/.yml file under basePath with its full content; visit returns stop=true
// to end the walk early. It returns whether the walk was stopped so recursion can
// unwind promptly.
func walkWorktreeFiles(wt *git.Worktree, basePath string, visit func(path string, content []byte) (stop bool, err error)) (bool, error) {
	files, err := wt.Filesystem.ReadDir(basePath)
	if err != nil {
		return false, fmt.Errorf("failed to read directory %s: %w", basePath, err)
	}

	for _, f := range files {
		var path string
		if basePath == "/" || basePath == "" {
			path = f.Name()
		} else {
			path = basePath + "/" + f.Name()
		}

		if f.IsDir() {
			stop, err := walkWorktreeFiles(wt, path, visit)
			if err != nil {
				return stop, err
			}
			if stop {
				return true, nil
			}
			continue
		}

		if !strings.HasSuffix(f.Name(), ".yaml") && !strings.HasSuffix(f.Name(), ".yml") {
			continue
		}

		content, err := readWorktreeFile(wt, path)
		if err != nil {
			return false, fmt.Errorf("failed to read %s: %w", path, err)
		}
		stop, err := visit(path, content)
		if err != nil {
			return true, err
		}
		if stop {
			return true, nil
		}
	}

	return false, nil
}

// -- File I/O helpers --------------------------------------------------------

// readWorktreeFile reads the whole content of path from the worktree filesystem.
func readWorktreeFile(wt *git.Worktree, path string) ([]byte, error) {
	f, err := wt.Filesystem.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

// writeWorktreeFile creates (truncating) path in the worktree, creating any
// missing parent directories, and writes content to it.
func writeWorktreeFile(wt *git.Worktree, path string, content []byte) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		if err := wt.Filesystem.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	f, err := wt.Filesystem.Create(path)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// removeWorktreeFile removes path from the worktree filesystem.
func removeWorktreeFile(wt *git.Worktree, path string) error {
	return wt.Filesystem.Remove(path)
}

// worktreeRelativePath turns a path produced by the walk into one relative to the
// worktree root (no leading slash), mirroring how claimed paths are recorded.
func worktreeRelativePath(root, path string) string {
	rel := strings.TrimPrefix(filepath.Clean(path), filepath.Clean(root))
	return strings.TrimPrefix(rel, "/")
}

// -- Document matching & rewriting -------------------------------------------

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

// replaceDocInContent replaces, within content, the first YAML document matching
// sel with newDoc and returns the merged content. When newDoc is empty the
// matching document is dropped (deletion). found is false (and content is
// returned unchanged) when no document matches; sibling documents are preserved
// verbatim.
func replaceDocInContent(content []byte, sel ObjectSelector, newDoc []byte) ([]byte, bool) {
	docs := bytes.Split(content, docSeparator)

	matched := -1
	for i, doc := range docs {
		if matchDoc(doc, sel) {
			matched = i
			break
		}
	}
	if matched == -1 {
		return content, false
	}

	if len(newDoc) == 0 {
		docs = append(docs[:matched], docs[matched+1:]...)
	} else {
		docs[matched] = bytes.TrimRight(newDoc, "\n")
	}

	merged := bytes.Join(docs, docSeparator)
	if len(merged) > 0 && !bytes.HasSuffix(merged, []byte("\n")) {
		merged = append(merged, '\n')
	}
	return merged, true
}

// appendDoc appends doc as a new YAML document at the end of existing, preserving
// the existing content.
func appendDoc(existing, doc []byte) []byte {
	out := bytes.TrimRight(existing, "\n")
	out = append(out, docSeparator...)
	out = append(out, doc...)
	if !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	return out
}

// selectorFromDoc derives a selector from a document's own Kubernetes identity so
// a write can locate "the document that equals me" within an existing file.
func selectorFromDoc(doc []byte) ObjectSelector {
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
