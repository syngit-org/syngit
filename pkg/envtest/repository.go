package envtest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage/memory"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

const defaultCommitterName = "syngit-envtest"
const defaultCommitterEmail = "envtest@syngit.io"

// repoPath returns the filesystem path of the bare repo for the given ref.
func (gs *GitServer) repoPath(repo RepoRef) string {
	return filepath.Join(gs.dir, repo.Owner, repo.Name+".git")
}

// CreateRepo creates a bare repository with an initial empty-tree commit
// on defaultBranch. Returns an error if the repository already exists.
func (gs *GitServer) CreateRepo(repo RepoRef, defaultBranch string) error {
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	path := gs.repoPath(repo)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("repository %s already exists", repo)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("failed to create repo dir: %w", err)
	}

	r, err := git.PlainInit(path, true)
	if err != nil {
		return fmt.Errorf("failed to init bare repo: %w", err)
	}

	if _, err := createInitialCommit(r, defaultBranch); err != nil {
		return fmt.Errorf("failed to create initial commit: %w", err)
	}
	return nil
}

// createInitialCommit writes an empty tree and a root commit pointing to
// it, then sets HEAD and refs/heads/{branch}.
func createInitialCommit(r *git.Repository, branch string) (plumbing.Hash, error) {
	store := r.Storer

	tree := &object.Tree{}
	treeEnc := store.NewEncodedObject()
	treeEnc.SetType(plumbing.TreeObject)
	if err := tree.Encode(treeEnc); err != nil {
		return plumbing.ZeroHash, err
	}
	treeHash, err := store.SetEncodedObject(treeEnc)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	sig := defaultSignature()
	commit := &object.Commit{
		Author:    sig,
		Committer: sig,
		Message:   "Initial commit\n",
		TreeHash:  treeHash,
	}
	commitEnc := store.NewEncodedObject()
	commitEnc.SetType(plumbing.CommitObject)
	if err := commit.Encode(commitEnc); err != nil {
		return plumbing.ZeroHash, err
	}
	commitHash, err := store.SetEncodedObject(commitEnc)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	branchRef := plumbing.NewBranchReferenceName(branch)
	if err := store.SetReference(plumbing.NewHashReference(branchRef, commitHash)); err != nil {
		return plumbing.ZeroHash, err
	}
	if err := store.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, branchRef)); err != nil {
		return plumbing.ZeroHash, err
	}
	return commitHash, nil
}

// CreateBranch creates a new branch pointing at the current HEAD of
// sourceBranch. Fails if the branch already exists.
func (gs *GitServer) CreateBranch(repo RepoRef, branchName, sourceBranch string) error {
	r, err := git.PlainOpen(gs.repoPath(repo))
	if err != nil {
		return err
	}
	newRef := plumbing.NewBranchReferenceName(branchName)
	if _, err := r.Reference(newRef, false); err == nil {
		return fmt.Errorf("branch %s already exists", branchName)
	}
	srcRef, err := r.Reference(plumbing.NewBranchReferenceName(sourceBranch), true)
	if err != nil {
		return fmt.Errorf("source branch %s: %w", sourceBranch, err)
	}
	return r.Storer.SetReference(plumbing.NewHashReference(newRef, srcRef.Hash()))
}

// DeleteBranch removes a branch reference from the repository.
func (gs *GitServer) DeleteBranch(repo RepoRef, branchName string) error {
	r, err := git.PlainOpen(gs.repoPath(repo))
	if err != nil {
		return err
	}
	return r.Storer.RemoveReference(plumbing.NewBranchReferenceName(branchName))
}

// CommitFile writes content at path on the given branch and commits it.
// The branch must already exist. Directories in path are created as needed.
func (gs *GitServer) CommitFile(repo RepoRef, branch, path string, content []byte, commitMsg string) error {
	return gs.CommitFiles(repo, branch, map[string][]byte{path: content}, commitMsg)
}

// CommitFiles writes the given files atomically in a single commit on the
// given branch. Paths are relative to the repo root and may contain slashes.
func (gs *GitServer) CommitFiles(repo RepoRef, branch string, files map[string][]byte, commitMsg string) error {
	bareRepoPath := gs.repoPath(repo)

	r, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:           bareRepoPath,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
	})
	if err != nil {
		return fmt.Errorf("clone for commit: %w", err)
	}

	wt, err := r.Worktree()
	if err != nil {
		return err
	}
	fs := wt.Filesystem

	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		if err := writeFile(fs, p, files[p]); err != nil {
			return err
		}
		if _, err := wt.Add(p); err != nil {
			return fmt.Errorf("add %s: %w", p, err)
		}
	}

	sig := defaultSignature()
	if _, err := wt.Commit(commitMsg, &git.CommitOptions{Author: &sig, Committer: &sig}); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	branchRef := plumbing.NewBranchReferenceName(branch)
	return r.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(branchRef + ":" + branchRef)},
	})
}

// CommitObject marshals a runtime.Object to YAML and commits it as a file.
func (gs *GitServer) CommitObject(repo RepoRef, branch, path string, obj runtime.Object, commitMsg string) error {
	content, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("marshal object: %w", err)
	}
	return gs.CommitFile(repo, branch, path, content, commitMsg)
}

// ReadFile reads the file at path on the given branch. Returns an error if
// the branch or file does not exist.
func (gs *GitServer) ReadFile(repo RepoRef, branch, path string) ([]byte, error) {
	r, err := git.PlainOpen(gs.repoPath(repo))
	if err != nil {
		return nil, err
	}
	ref, err := r.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return nil, fmt.Errorf("branch %s: %w", branch, err)
	}
	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	f, err := tree.File(path)
	if err != nil {
		return nil, err
	}
	reader, err := f.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close() // nolint:errcheck
	return io.ReadAll(reader)
}

// FileExists returns true if the file exists at path on the given branch.
func (gs *GitServer) FileExists(repo RepoRef, branch, path string) (bool, error) {
	r, err := git.PlainOpen(gs.repoPath(repo))
	if err != nil {
		return false, err
	}
	ref, err := r.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return false, fmt.Errorf("branch %s: %w", branch, err)
	}
	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return false, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return false, err
	}
	_, err = tree.File(path)
	if err == object.ErrFileNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListFiles returns all file paths (blobs) on the given branch.
func (gs *GitServer) ListFiles(repo RepoRef, branch string) ([]string, error) {
	r, err := git.PlainOpen(gs.repoPath(repo))
	if err != nil {
		return nil, err
	}
	ref, err := r.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return nil, fmt.Errorf("branch %s: %w", branch, err)
	}
	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	var files []string
	err = tree.Files().ForEach(func(f *object.File) error {
		files = append(files, f.Name)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// MergeBranch merges sourceBranch into targetBranch. Performs a fast-forward
// when possible; otherwise creates a merge commit whose tree is the union of
// both branches (source wins for conflicting paths, subtrees are merged
// recursively).
func (gs *GitServer) MergeBranch(repo RepoRef, sourceBranch, targetBranch string) error {
	r, err := git.PlainOpen(gs.repoPath(repo))
	if err != nil {
		return err
	}
	sourceRef, err := r.Reference(plumbing.NewBranchReferenceName(sourceBranch), true)
	if err != nil {
		return fmt.Errorf("source branch %s: %w", sourceBranch, err)
	}
	targetRef, err := r.Reference(plumbing.NewBranchReferenceName(targetBranch), true)
	if err != nil {
		return fmt.Errorf("target branch %s: %w", targetBranch, err)
	}

	if sourceRef.Hash() == targetRef.Hash() {
		return nil
	}

	sourceCommit, err := r.CommitObject(sourceRef.Hash())
	if err != nil {
		return err
	}
	targetCommit, err := r.CommitObject(targetRef.Hash())
	if err != nil {
		return err
	}

	canFF, err := targetCommit.IsAncestor(sourceCommit)
	if err != nil {
		return err
	}
	targetBranchRef := plumbing.NewBranchReferenceName(targetBranch)
	if canFF {
		return r.Storer.SetReference(plumbing.NewHashReference(targetBranchRef, sourceRef.Hash()))
	}

	mergedTreeHash, err := mergeTreeObjects(r.Storer, targetCommit.TreeHash, sourceCommit.TreeHash)
	if err != nil {
		return fmt.Errorf("merge trees: %w", err)
	}

	sig := defaultSignature()
	mergeCommit := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      fmt.Sprintf("Merge branch '%s' into %s\n", sourceBranch, targetBranch),
		TreeHash:     mergedTreeHash,
		ParentHashes: []plumbing.Hash{targetRef.Hash(), sourceRef.Hash()},
	}
	enc := r.Storer.NewEncodedObject()
	enc.SetType(plumbing.CommitObject)
	if err := mergeCommit.Encode(enc); err != nil {
		return err
	}
	hash, err := r.Storer.SetEncodedObject(enc)
	if err != nil {
		return err
	}
	return r.Storer.SetReference(plumbing.NewHashReference(targetBranchRef, hash))
}

// mergeTreeObjects creates a new tree that is the union of base and overlay.
// Overlay entries override base entries for the same name. When both have a
// directory entry with the same name, the subtrees are merged recursively.
func mergeTreeObjects(s storer.EncodedObjectStorer, baseHash, overlayHash plumbing.Hash) (plumbing.Hash, error) {
	if baseHash == overlayHash {
		return baseHash, nil
	}

	baseEntries, err := decodeTreeEntries(s, baseHash)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("decode base tree: %w", err)
	}
	overlayEntries, err := decodeTreeEntries(s, overlayHash)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("decode overlay tree: %w", err)
	}

	merged := make(map[string]object.TreeEntry)
	for _, e := range baseEntries {
		merged[e.Name] = e
	}
	for _, e := range overlayEntries {
		if existing, ok := merged[e.Name]; ok && existing.Mode == filemode.Dir && e.Mode == filemode.Dir {
			sub, err := mergeTreeObjects(s, existing.Hash, e.Hash)
			if err != nil {
				return plumbing.ZeroHash, err
			}
			merged[e.Name] = object.TreeEntry{Name: e.Name, Mode: e.Mode, Hash: sub}
		} else {
			merged[e.Name] = e
		}
	}

	entries := make([]object.TreeEntry, 0, len(merged))
	for _, e := range merged {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	newTree := &object.Tree{Entries: entries}
	enc := s.NewEncodedObject()
	enc.SetType(plumbing.TreeObject)
	if err := newTree.Encode(enc); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("encode merged tree: %w", err)
	}
	return s.SetEncodedObject(enc)
}

func decodeTreeEntries(s storer.EncodedObjectStorer, hash plumbing.Hash) ([]object.TreeEntry, error) {
	enc, err := s.EncodedObject(plumbing.TreeObject, hash)
	if err != nil {
		return nil, err
	}
	t := &object.Tree{}
	if err := t.Decode(enc); err != nil {
		return nil, err
	}
	return t.Entries, nil
}

// GetLatestCommit returns the hash and message of the latest commit on branch.
func (gs *GitServer) GetLatestCommit(repo RepoRef, branch string) (hash, message string, err error) {
	r, err := git.PlainOpen(gs.repoPath(repo))
	if err != nil {
		return "", "", err
	}
	ref, err := r.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return "", "", fmt.Errorf("branch %s: %w", branch, err)
	}
	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return "", "", err
	}
	return commit.Hash.String(), commit.Message, nil
}

// writeFile creates (or replaces) a file in a billy filesystem. memfs
// auto-creates parent directories.
func writeFile(fs billy.Filesystem, path string, content []byte) error {
	f, err := fs.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

func defaultSignature() object.Signature {
	return object.Signature{
		Name:  defaultCommitterName,
		Email: defaultCommitterEmail,
		When:  time.Now(),
	}
}
