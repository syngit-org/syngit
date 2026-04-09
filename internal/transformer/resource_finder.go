package transformer

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	yaml "k8s.io/apimachinery/pkg/util/yaml"
)

type ResourceFinder struct {
	searchedGVK       schema.GroupVersionResource
	searchedName      string
	searchedNamespace string
	content           string
}

func (rf ResourceFinder) Transform(params syngit.GitPipelineParams, worktree *git.Worktree) (*git.Worktree, syngit.ModifiedPaths, error) {
	rf.searchedGVK = params.InterceptedGVR
	rf.searchedName = params.InterceptedName
	rf.searchedNamespace = params.RemoteSyncer.Namespace
	rf.content = params.InterceptedYAML

	if params.RemoteSyncer.Spec.ResourceFinder {
		modifiedPaths, err := rf.getPathsContent(worktree, worktree.Filesystem.Root())
		if err != nil {
			return worktree, modifiedPaths, err
		}

		return worktree, modifiedPaths, nil
	}
	return nil, syngit.NewModifiedPaths(), nil
}

func (rf ResourceFinder) getPathsContent(worktree *git.Worktree, basePath string) (syngit.ModifiedPaths, error) {
	modifiedPaths := syngit.NewModifiedPaths()

	files, err := worktree.Filesystem.ReadDir(basePath)
	if err != nil {
		return syngit.NewModifiedPaths(), fmt.Errorf("failed to read directory %s: %w", basePath, err)
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
				return syngit.NewModifiedPaths(), err
			}
			modifiedPaths.AppendModifiedPaths(paths)
		} else {
			if strings.HasSuffix(currentFileName, ".yaml") || strings.HasSuffix(currentFileName, ".yml") {

				paths, err := rf.checkInsertResource(worktree, path)
				if err != nil {
					return syngit.NewModifiedPaths(), err
				}
				modifiedPaths.AppendModifiedPaths(paths)
			}
		}
	}

	return modifiedPaths, nil
}

func (rf ResourceFinder) checkInsertResource(wt *git.Worktree, path string) (syngit.ModifiedPaths, error) {
	modifiedPaths := syngit.NewModifiedPaths()

	f, err := wt.Filesystem.Open(path)
	if err != nil {
		return modifiedPaths, fmt.Errorf("failed to open the %s file in the worktree: %w", path, err)
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return modifiedPaths, fmt.Errorf("failed to read the %s file in the worktree: %w", path, err)
	}

	err = f.Close()
	if err != nil {
		return modifiedPaths, fmt.Errorf("failed to close the %s file in the worktree: %w", path, err)
	}

	out := rf.replaceResourceIfFound(content)
	if string(out) != string(content) {
		// Remove the file first to ensure clean state
		_ = wt.Filesystem.Remove(path)

		if len(out) > 0 {
			file, err := wt.Filesystem.Create(path)
			if err != nil {
				return modifiedPaths, fmt.Errorf("failed to create the %s file in the worktree: %w", path, err)
			}

			_, err = file.Write(out)
			if err != nil {
				_ = file.Close()
				return modifiedPaths, fmt.Errorf("failed to write the %s file in the worktree: %w", path, err)
			}
			err = file.Close()
			if err != nil {
				return modifiedPaths, fmt.Errorf("failed to close the %s file in the worktree: %w", path, err)
			}
		}

		// Clean the path to remove any leading slashes and normalize it
		cleanPath := filepath.Clean(path)
		if after, ok := strings.CutPrefix(cleanPath, "/"); ok {
			cleanPath = after
		}

		modifiedPaths.AppendAddedPath(cleanPath)
	}

	return modifiedPaths, nil
}

func (rf ResourceFinder) replaceResourceIfFound(content []byte) []byte {
	targetGVK := fmt.Sprintf("%s/%s", rf.searchedGVK.Group, rf.searchedGVK.Version)
	if rf.searchedGVK.Group == "" {
		targetGVK = rf.searchedGVK.Version
	}

	// Split content into raw document strings, preserving original formatting
	rawDocs := bytes.Split(content, []byte("\n---\n"))
	if len(rawDocs) == 0 {
		return content
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

	documentFound := false
	// Walk docs and check metadata
	for i, doc := range docs {
		if doc.apiVersion == "" && doc.kind == "" {
			// Skip unparseable or empty docs
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
			docs[i].rawBytes = []byte(rf.content)
			break
		}
	}

	if !documentFound {
		return content
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

	return []byte(out.String())
}
