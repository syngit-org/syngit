// Package walker manipulates Kubernetes/YAML documents inside a git worktree.
// Its responsibilities are split across files by concern:
//   - walker.go      the recursive worktree walk (WalkWorktreeYAML, walkWorktreeFiles),
//   - worktree_fs.go reading/writing/removing worktree files,
//   - selector.go    the ObjectSelector type and document matching,
//   - document.go    in-memory replacement/append of YAML documents,
//   - object.go      the public object-level API (FindObject, ReplaceObject,
//     WriteObjectAtPath) layered on top of the above.
package walker

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
)

// docSeparator is the canonical YAML document separator used everywhere to split
// and rejoin multi-document worktree files.
var docSeparator = []byte("\n---\n")

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
