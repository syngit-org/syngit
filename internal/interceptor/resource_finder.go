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
	syaml "sigs.k8s.io/yaml"
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

	out, err := rf.replaceResourceIfFound(content)
	if err != nil {
		return err
	}
	if string(out) != string(content) {
		// Remove the file first to ensure clean state
		_ = wt.Filesystem.Remove(path)

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

		// Clean the path to remove any leading slashes and normalize it
		cleanPath := filepath.Clean(path)
		if after, ok := strings.CutPrefix(cleanPath, "/"); ok {
			cleanPath = after
		}
		rf.paths = append(rf.paths, cleanPath)
	}

	return nil
}

func (rf *ResourceFinder) replaceResourceIfFound(content []byte) ([]byte, error) {
	targetGVK := fmt.Sprintf("%s/%s", rf.SearchedGVK.Group, rf.SearchedGVK.Version)
	if rf.SearchedGVK.Group == "" {
		targetGVK = rf.SearchedGVK.Version
	}

	var docs [][]byte

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(content), 4096)

	for {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return []byte{}, fmt.Errorf("can't decode content: %w", err)
		}
		if len(raw) == 0 {
			// skip empty docs
			continue
		}

		// Simpler: convert map back to YAML
		y, err := syaml.Marshal(raw)
		if err != nil {
			return []byte{}, fmt.Errorf("marshal back failed: %w", err)
		}
		docs = append(docs, y)
	}

	// Walk docs and check metadata
	for i, doc := range docs {
		var raw map[string]interface{}
		if err := yaml.Unmarshal(doc, &raw); err != nil {
			return []byte{}, fmt.Errorf("unmarshal failed: %w", err)
		}

		apiVersion, _ := raw["apiVersion"].(string)
		k, _ := raw["kind"].(string)

		md, _ := raw["metadata"].(map[string]interface{})
		n, _ := md["name"].(string)
		ns, _ := md["namespace"].(string)

		// Convert Kind (singular) to Resource (plural) for comparison
		gvk := schema.GroupVersionKind{
			Group:   rf.SearchedGVK.Group,
			Version: rf.SearchedGVK.Version,
			Kind:    k,
		}
		resourceFromKind, _ := meta.UnsafeGuessKindToResource(gvk)

		if apiVersion == targetGVK &&
			resourceFromKind.Resource == rf.SearchedGVK.Resource &&
			n == rf.SearchedName &&
			(rf.SearchedNamespace == "" || ns == rf.SearchedNamespace) {
			docs[i] = bytes.TrimSpace([]byte(rf.Content))
			break
		}
	}

	// Reassemble docs with proper separators
	var out strings.Builder
	for i, d := range docs {
		if i > 0 {
			out.WriteString("---\n")
		}
		out.Write(d)
		if !strings.HasSuffix(string(d), "\n") {
			out.WriteString("\n")
		}
	}

	return []byte(out.String()), nil
}
