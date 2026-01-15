package interceptor

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	yaml "k8s.io/apimachinery/pkg/util/yaml"
)

type ResourceFinder struct {
	SearchedGVK       schema.GroupVersionResource
	SearchedName      string
	SearchedNamespace string
	Content           string
	paths             []string
}

type ResourceFinderResults struct {
	Found bool
	Paths []string
}

func (rf *ResourceFinder) BuildWorktree(wt *git.Worktree) (ResourceFinderResults, error) {
	rfr := ResourceFinderResults{Found: false, Paths: []string{}}
	rf.paths = []string{}

	err := rf.getPathsContent(wt, wt.Filesystem.Root())
	if err != nil {
		return rfr, err
	}

	if len(rf.paths) > 0 {
		rfr.Found = true
		rfr.Paths = rf.paths
	}

	return rfr, nil
}

func (rf *ResourceFinder) getPathsContent(wt *git.Worktree, basePath string) error {

	files, err := wt.Filesystem.ReadDir(basePath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", basePath, err)
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
			err = rf.getPathsContent(wt, path)
			if err != nil {
				return err
			}
		} else {
			if strings.HasSuffix(currentFileName, ".yaml") || strings.HasSuffix(currentFileName, ".yml") {

				err = rf.checkInsertResource(wt, path)
				if err != nil {
					return err
				}

			}
		}

	}

	return nil
}

type TypeMeta struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

type ObjectMeta struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

type GenericK8sObject struct {
	TypeMeta   `yaml:",inline"`
	ObjectMeta `yaml:"metadata"`
}

func (rf *ResourceFinder) checkInsertResource(wt *git.Worktree, path string) error {
	f, err := wt.Filesystem.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open the %s file in the worktree: %w", path, err)
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed to read the %s file in the worktree: %w", path, err)
	}

	err = f.Close()
	if err != nil {
		return fmt.Errorf("failed to close the %s file in the worktree: %w", path, err)
	}

	out := rf.replaceResourceIfFound(content)
	if string(out) != string(content) {
		// Remove the file first to ensure clean state
		_ = wt.Filesystem.Remove(path)

		if len(out) > 0 {
			file, err := wt.Filesystem.Create(path)
			if err != nil {
				return fmt.Errorf("failed to create the %s file in the worktree: %w", path, err)
			}

			_, err = file.Write(out)
			if err != nil {
				_ = file.Close()
				return fmt.Errorf("failed to write the %s file in the worktree: %w", path, err)
			}
			err = file.Close()
			if err != nil {
				return fmt.Errorf("failed to close the %s file in the worktree: %w", path, err)
			}
		}

		// Clean the path to remove any leading slashes and normalize it
		cleanPath := filepath.Clean(path)
		if after, ok := strings.CutPrefix(cleanPath, "/"); ok {
			cleanPath = after
		}
		rf.paths = append(rf.paths, cleanPath)
	}

	return nil
}

func (rf *ResourceFinder) replaceResourceIfFound(content []byte) []byte {
	targetGVK := fmt.Sprintf("%s/%s", rf.SearchedGVK.Group, rf.SearchedGVK.Version)
	if rf.SearchedGVK.Group == "" {
		targetGVK = rf.SearchedGVK.Version
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
			Version: rf.SearchedGVK.Version,
			Kind:    doc.kind,
		}
		resourceFromKind, _ := meta.UnsafeGuessKindToResource(gvk)

		if doc.apiVersion == targetGVK &&
			resourceFromKind.Resource == rf.SearchedGVK.Resource &&
			doc.name == rf.SearchedName &&
			(rf.SearchedNamespace == "" || doc.namespace == rf.SearchedNamespace) {
			documentFound = true
			// Replace only the matched document
			docs[i].rawBytes = []byte(rf.Content)
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
