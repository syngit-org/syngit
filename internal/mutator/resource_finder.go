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
	yaml "k8s.io/apimachinery/pkg/util/yaml"
)

type ResourceFinder struct{}

type resourceFinderImplem struct {
	searchedGVK       schema.GroupVersionResource
	searchedName      string
	searchedNamespace string
	content           []byte
}

func (rf ResourceFinder) Customize(params interceptor.GitPipelineParams, mutations Mutations, customWorktree *CustomWorktree) error {
	for gvr, content := range mutations {
		searchedName := params.InterceptedName
		searchedNamespace := params.RemoteSyncer.Namespace

		resourceFinder := resourceFinderImplem{
			searchedName:      searchedName,
			searchedNamespace: searchedNamespace,
			searchedGVK:       gvr,
			content:           content,
		}

		if params.RemoteSyncer.Spec.ResourceFinder {
			claimedPaths, err := resourceFinder.getPathsContent(customWorktree.Worktree, customWorktree.Worktree.Filesystem.Root())
			if err != nil {
				return err
			}
			customWorktree.ClaimedPaths = claimedPaths

			return nil
		}
	}

	return nil
}

func (rf resourceFinderImplem) getPathsContent(worktree *git.Worktree, basePath string) (interceptor.ClaimedPaths, error) {
	claimedPaths := interceptor.NewClaimedPaths()

	files, err := worktree.Filesystem.ReadDir(basePath)
	if err != nil {
		return interceptor.NewClaimedPaths(), fmt.Errorf("failed to read directory %s: %w", basePath, err)
	}

	var path string
	var currentFileName string

	for _, f := range files {
		currentFileName = f.Name()
		if basePath == "/" || basePath == "" {
			path = currentFileName
		} else {
			path = basePath + "/" + currentFileName
		}

		if f.IsDir() {
			paths, err := rf.getPathsContent(worktree, path)
			if err != nil {
				return interceptor.NewClaimedPaths(), err
			}
			claimedPaths.AppendClaimedPaths(paths)
		} else {
			if strings.HasSuffix(currentFileName, ".yaml") || strings.HasSuffix(currentFileName, ".yml") {

				paths, err := rf.checkInsertResource(worktree, path)
				if err != nil {
					return interceptor.NewClaimedPaths(), err
				}
				claimedPaths.AppendClaimedPaths(paths)
			}
		}
	}

	return claimedPaths, nil
}

func (rf resourceFinderImplem) checkInsertResource(wt *git.Worktree, path string) (interceptor.ClaimedPaths, error) {
	claimedPaths := interceptor.NewClaimedPaths()

	f, err := wt.Filesystem.Open(path)
	if err != nil {
		return claimedPaths, fmt.Errorf("failed to open the %s file in the worktree: %w", path, err)
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return claimedPaths, fmt.Errorf("failed to read the %s file in the worktree: %w", path, err)
	}

	err = f.Close()
	if err != nil {
		return claimedPaths, fmt.Errorf("failed to close the %s file in the worktree: %w", path, err)
	}

	out, found := rf.replaceResourceIfFound(content)
	if !found {
		return claimedPaths, nil
	}

	cleanPath := filepath.Clean(path)
	if after, ok := strings.CutPrefix(cleanPath, "/"); ok {
		cleanPath = after
	}

	if string(out) != string(content) {
		// Remove the file first to ensure clean state
		_ = wt.Filesystem.Remove(path)

		if len(out) > 0 {
			file, err := wt.Filesystem.Create(path)
			if err != nil {
				return claimedPaths, fmt.Errorf("failed to create the %s file in the worktree: %w", path, err)
			}

			_, err = file.Write(out)
			if err != nil {
				_ = file.Close()
				return claimedPaths, fmt.Errorf("failed to write the %s file in the worktree: %w", path, err)
			}
			err = file.Close()
			if err != nil {
				return claimedPaths, fmt.Errorf("failed to close the %s file in the worktree: %w", path, err)
			}
		}
	}

	// Record the match even when content didn't change, so the caller knows
	// ResourceFinder claimed this resource and the fallback should not fire.

	if string(rf.content) == "" {
		// Empty comment so the path should be deleted.
		claimedPaths.AppendDeletedPath(cleanPath)
	} else {
		claimedPaths.AppendAddedPath(cleanPath)
	}

	return claimedPaths, nil
}

// replaceResourceIfFound scans content for a YAML document that matches the
// configured search identity. It returns the (possibly rewritten) content and
// a boolean indicating whether a match was found. When found=true the caller
// should claim the file even if the returned content equals the input. A no-op
// rewrite still counts as a match.
func (rf resourceFinderImplem) replaceResourceIfFound(content []byte) ([]byte, bool) {
	targetGVK := fmt.Sprintf("%s/%s", rf.searchedGVK.Group, rf.searchedGVK.Version)
	if rf.searchedGVK.Group == "" {
		targetGVK = rf.searchedGVK.Version
	}

	// Split content into raw document strings, preserving original formatting
	rawDocs := bytes.Split(content, []byte("\n---\n"))
	if len(rawDocs) == 0 {
		return content, false
	}

	// Track which documents are valid and their parsed metadata
	type docInfo struct {
		rawBytes   []byte
		apiVersion string
		kind       string
		name       string
		namespace  string
	}
	var docs = []docInfo{}
	documentFound := false

	for i, rawDoc := range rawDocs {
		// Trim leading/trailing whitespace but preserve the original doc
		trimmed := bytes.TrimSpace(rawDoc)
		if len(trimmed) == 0 {
			continue
		}

		// For first doc, handle potential leading "---"
		if i == 0 {
			if after, ok := bytes.CutPrefix(trimmed, []byte("---")); ok {
				trimmed = bytes.TrimSpace(after)
			}
		}

		// Parse just enough to check if it matches
		var raw map[string]interface{}
		if err := yaml.Unmarshal(trimmed, &raw); err != nil {
			// If we can't parse it, keep the original bytes
			docs = append(docs, docInfo{rawBytes: rawDoc})
			continue
		}

		if len(raw) == 0 {
			// Empty document, keep original
			docs = append(docs, docInfo{rawBytes: rawDoc})
			continue
		}

		apiVersion, _ := raw["apiVersion"].(string)
		k, _ := raw["kind"].(string)

		md, _ := raw["metadata"].(map[string]interface{})
		n, _ := md["name"].(string)
		ns, _ := md["namespace"].(string)

		docs = append(docs, docInfo{
			rawBytes:   rawDoc,
			apiVersion: apiVersion,
			kind:       k,
			name:       n,
			namespace:  ns,
		})
	}

	// Walk docs and check metadata
	for i, doc := range docs {
		if doc.apiVersion == "" && doc.kind == "" {
			// Not a Kubernetes manifest. May still be managed by a Syngit
			// provider if the first line carries the marker.
			firstLine, _, _ := bytes.Cut(doc.rawBytes, []byte("\n"))
			markerLine := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(firstLine), []byte("#")))
			if !bytes.HasPrefix(markerLine, []byte(ResourceFinderCommentPrefix)) {
				continue
			}
			value := string(bytes.TrimPrefix(markerLine, []byte(ResourceFinderCommentPrefix)))
			ns, name, hasSep := strings.Cut(value, "/")
			if !hasSep {
				name = ns
				ns = ""
			}

			if name == rf.searchedName && (rf.searchedNamespace == "" || ns == rf.searchedNamespace) {
				docs[i].rawBytes = rf.content
				documentFound = true
				break
			}
			continue
		}

		// Convert Kind (singular) to Resource (plural) for comparison
		gvk := schema.GroupVersionKind{
			Group:   doc.apiVersion,
			Version: rf.searchedGVK.Version,
			Kind:    doc.kind,
		}
		resourceFromKind, _ := meta.UnsafeGuessKindToResource(gvk)

		if doc.apiVersion == targetGVK &&
			resourceFromKind.Resource == rf.searchedGVK.Resource &&
			doc.name == rf.searchedName &&
			(rf.searchedNamespace == "" || doc.namespace == rf.searchedNamespace) {
			documentFound = true
			// Replace only the matched document
			docs[i].rawBytes = rf.content
			break
		}
	}

	if !documentFound {
		return content, false
	}

	// Reassemble docs with proper separators, preserving original formatting
	var out strings.Builder
	for i, doc := range docs {
		if i > 0 {
			out.WriteString("---\n")
		}
		out.Write(bytes.TrimSpace(doc.rawBytes))
		if len(docs[i].rawBytes) > 0 && !bytes.HasSuffix(bytes.TrimSpace(doc.rawBytes), []byte("\n")) {
			out.WriteString("\n")
		}
	}

	return []byte(out.String()), true
}
