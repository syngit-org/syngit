package mutator

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5"
)

// WalkWorktreeYAML recursively visits every .yaml/.yml file under basePath and
// calls visit with the file path and its content. When visit returns stop=true
// the walk ends early.
func WalkWorktreeYAML(worktree *git.Worktree, basePath string, visit func(path string, content []byte) (stop bool, skip bool, err error)) error {
	files, err := worktree.Filesystem.ReadDir(basePath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", basePath, err)
	}

	for _, f := range files {
		var path string
		if basePath == "/" || basePath == "" {
			path = f.Name()
		} else {
			path = basePath + "/" + f.Name()
		}

		if f.IsDir() {
			if err := WalkWorktreeYAML(worktree, path, visit); err != nil {
				return err
			}
			continue
		}

		if !strings.HasSuffix(f.Name(), ".yaml") && !strings.HasSuffix(f.Name(), ".yml") {
			continue
		}

		file, err := worktree.Filesystem.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", path, err)
		}
		content, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		for _, rawDoc := range bytes.Split(content, []byte("---")) {
			stop, skip, err := visit(path, rawDoc)
			if err != nil {
				return err
			}
			if skip {
				continue
			}
			if stop {
				return nil
			}
		}
	}

	return nil
}
