package walker

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/syngit-org/syngit/pkg/interceptor"
)

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

// ReplaceObject walks the worktree, replaces the first document matching sel with
// content (deletes it when content is empty), writes the file back preserving
// sibling documents, and returns the claimed paths. It claims nothing when no
// document matches.
func ReplaceObject(wt *git.Worktree, sel ObjectSelector, content []byte) (interceptor.ClaimedPaths, error) {
	claimed := interceptor.NewClaimedPaths()
	root := wt.Filesystem.Root()

	_, err := walkWorktreeFiles(wt, root, func(path string, fileContent []byte) (bool, error) {
		out, found := ReplaceDocInContent(fileContent, sel, content)
		if !found {
			return false, nil
		}

		if string(out) != string(fileContent) {
			if len(bytes.TrimSpace(out)) == 0 {
				if rerr := removeWorktreeFile(wt, path); rerr != nil {
					return true, fmt.Errorf("failed to remove %s: %w", path, rerr)
				}
			} else if werr := WriteWorktreeFile(wt, path, out); werr != nil {
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
		if merged, found := ReplaceDocInContent(existing, sel, content); found {
			out = merged
		} else {
			out = appendDoc(existing, content)
		}
	}

	if err := WriteWorktreeFile(wt, cleanPath, out); err != nil {
		return interceptor.NewClaimedPaths(), err
	}
	claimed.AppendAddedPath(cleanPath)
	return claimed, nil
}
