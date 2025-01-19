package interceptor

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
)

type GitConsistency struct {
	strategy           syngit.ConsistencyStrategy
	targetRepository   *git.Repository
	upstreamRepository *git.Repository
}

func getRepository(gp GitPusher, repo string, branch string) (*git.Repository, error) {
	// Clone the repository into memory
	var verboseOutput bytes.Buffer
	cloneOption := &git.CloneOptions{
		URL:           repo,
		ReferenceName: plumbing.ReferenceName(branch),
		Auth: &http.BasicAuth{
			Username: gp.gitUser,
			Password: gp.gitToken,
		},
		SingleBranch:    true,
		InsecureSkipTLS: gp.remoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if gp.caBundle != nil {
		cloneOption.CABundle = gp.caBundle
	}
	repository, err := git.Clone(memory.NewStorage(), memfs.New(), cloneOption)
	if err != nil {
		variables := fmt.Sprintf("\nRepository: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
			repo,
			plumbing.ReferenceName(branch),
			gp.gitUser,
			gp.gitEmail,
		)
		errMsg := fmt.Sprintf("failed to clone repository: %s\nVerbose output: %s\nVariables: %s\n", err.Error(), verboseOutput.String(), variables)
		return nil, errors.New(errMsg)
	}

	return repository, nil
}

func GetUpstreamRepository(gp GitPusher) (*git.Repository, error) {
	return getRepository(gp, gp.remoteTarget.Spec.UpstreamRepository, gp.remoteTarget.Spec.UpstreamBranch)
}

func GetTargetRepository(gp GitPusher) (*git.Repository, error) {
	return getRepository(gp, gp.remoteTarget.Spec.TargetRepository, gp.remoteTarget.Spec.TargetBranch)
}

func (gc GitConsistency) GetWorkTree() (*git.Worktree, error) {

	if gc.strategy == syngit.TryRebaseOrOverwrite {
		wt, err := gc.upstreamBasedRebase()
		if err != nil {
			wt, err = gc.upstreamBasedHardReset()
			if err != nil {
				return nil, err
			}
			return wt, nil
		}
		return wt, nil
	}

	if gc.strategy == syngit.Overwrite {
		wt, err := gc.upstreamBasedHardReset()
		if err != nil {
			return nil, err
		}
		return wt, nil
	}

	if gc.strategy == syngit.TryRebaseOrDie {
		wt, err := gc.upstreamBasedRebase()
		if err != nil {
			return nil, err
		}
		return wt, nil
	}

	return nil, fmt.Errorf("wrong target strategy; got %s", gc.strategy)
}

func (gc GitConsistency) upstreamBasedHardReset() (*git.Worktree, error) {
	// Get the upstream repository's reference to HEAD
	upstreamRef, err := gc.upstreamRepository.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream HEAD: %w", err)
	}

	// Fetch the latest upstream commit
	upstreamCommit, err := gc.upstreamRepository.CommitObject(upstreamRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream commit: %w", err)
	}

	// Get the worktree for the target repository
	worktree, err := gc.targetRepository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	// Perform a hard reset to the upstream commit
	if err := worktree.Reset(&git.ResetOptions{
		Commit: upstreamCommit.Hash,
		Mode:   git.HardReset,
	}); err != nil {
		return nil, fmt.Errorf("failed to hard reset: %w", err)
	}

	return worktree, nil
}

func (gc GitConsistency) upstreamBasedRebase() (*git.Worktree, error) {
	// Get the HEAD reference for the upstream repository
	upstreamRef, err := gc.upstreamRepository.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream HEAD: %w", err)
	}

	// Get the latest upstream commit
	upstreamCommit, err := gc.upstreamRepository.CommitObject(upstreamRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream commit: %w", err)
	}

	// Get the worktree for the target repository
	worktree, err := gc.targetRepository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	// Create an iterator for the upstream commits
	upstreamIter, err := gc.upstreamRepository.Log(&git.LogOptions{From: upstreamCommit.Hash})
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream commit log: %w", err)
	}

	defer upstreamIter.Close()

	// Cherry-pick each upstream commit onto the target repository
	err = upstreamIter.ForEach(func(c *object.Commit) error {
		// Check if the commit is already in the target repository
		_, err := gc.targetRepository.CommitObject(c.Hash)
		if err == nil {
			// Commit already exists, skip
			return nil
		}

		// Apply the commit to the target repository
		err = worktree.Checkout(&git.CheckoutOptions{
			Hash:  c.Hash,
			Force: true,
		})
		if err != nil {
			return fmt.Errorf("failed to cherry-pick commit %s: %w", c.Hash, err)
		}

		// Commit the changes
		_, err = worktree.Commit(c.Message, &git.CommitOptions{
			Author: &c.Author,
		})
		if err != nil {
			return fmt.Errorf("failed to commit changes for %s: %w", c.Hash, err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed during rebase: %w", err)
	}

	return worktree, nil
}
