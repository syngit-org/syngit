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

// ReplaceObject replaces the document matching sel with content (deletes it when
// content is empty) in every worktree file that contains it, writing each file
// back preserving its sibling documents, and returns the claimed paths. It claims
// nothing when no document matches.
//
// scope is an identifier for the worktree's content. When the document path cache
// is enabled it remembers, per (scope, sel), the single file that held the document.
// The cached path is always validated by re-reading it. A stale entry simply falls
// back to a full walk. Keys that have matched more than one file are never cached,
// so duplicated resources keep being rewritten in every copy.
func ReplaceObject(wt *git.Worktree, scope string, sel ObjectSelector, content []byte) (interceptor.ClaimedPaths, error) {
	key := docCacheKey{Scope: scope, Sel: sel}

	// Fast path: a single remembered location, validated by re-reading it.
	if v, ok := docCache.Get(key); ok && !v.NoCache {
		claimed := interceptor.NewClaimedPaths()
		if fileContent, rerr := readWorktreeFile(wt, v.Path); rerr == nil {
			found, aerr := applyReplacement(wt, v.Path, v.Path, fileContent, sel, content, &claimed)
			if aerr != nil {
				return interceptor.NewClaimedPaths(), aerr
			}
			if found {
				if len(content) == 0 {
					// A deletion empties the location; forget it.
					docCache.Delete(key)
				}
				return claimed, nil
			}
		}
		// File gone or no longer matching: fall back to a full walk, which
		// refreshes the cache below.
	}

	// Full walk: rewrite the document in every matching file.
	claimed := interceptor.NewClaimedPaths()
	root := wt.Filesystem.Root()
	var matched []string
	_, err := walkWorktreeFiles(wt, root, func(path string, fileContent []byte) (bool, error) {
		rel := worktreeRelativePath(root, path)
		found, aerr := applyReplacement(wt, path, rel, fileContent, sel, content, &claimed)
		if aerr != nil {
			return true, aerr
		}
		if found {
			matched = append(matched, rel)
		}
		return false, nil // keep walking: a later file may match too
	})
	if err != nil {
		return interceptor.NewClaimedPaths(), err
	}

	if len(content) == 0 {
		docCache.Delete(key) // nothing remains to point at after a deletion
	} else {
		recordMatches(scope, sel, matched)
	}
	return claimed, nil
}

// applyReplacement replaces the document matching sel inside fileContent.
// found reports whether a matching document was present; when false nothing
// is written or claimed.
func applyReplacement(wt *git.Worktree, fsPath, relPath string, fileContent []byte, sel ObjectSelector, content []byte, claimed *interceptor.ClaimedPaths) (bool, error) {
	out, found := ReplaceDocInContent(fileContent, sel, content)
	if !found {
		return false, nil
	}

	if string(out) != string(fileContent) {
		if len(bytes.TrimSpace(out)) == 0 {
			if rerr := removeWorktreeFile(wt, fsPath); rerr != nil {
				return true, fmt.Errorf("failed to remove %s: %w", fsPath, rerr)
			}
		} else if werr := WriteWorktreeFile(wt, fsPath, out); werr != nil {
			return true, fmt.Errorf("failed to write %s: %w", fsPath, werr)
		}
	}

	if len(content) == 0 {
		claimed.AppendDeletedPath(relPath)
	} else {
		claimed.AppendAddedPath(relPath)
	}
	return true, nil
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
