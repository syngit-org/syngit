package walker

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
)

// readWorktreeFile reads the whole content of path from the worktree filesystem.
func readWorktreeFile(wt *git.Worktree, path string) ([]byte, error) {
	f, err := wt.Filesystem.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

// WriteWorktreeFile creates (truncating) path in the worktree, creating any
// missing parent directories, and writes content to it.
func WriteWorktreeFile(wt *git.Worktree, path string, content []byte) error {
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
