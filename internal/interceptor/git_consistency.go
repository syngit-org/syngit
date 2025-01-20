package interceptor

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
)

type GitConsistency struct {
	strategy           syngit.MergeStrategy
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

	if gc.strategy == syngit.TryMergeCommitOrHardReset {
		wt, err := gc.upstreamBasedPull()
		if err != nil {
			wt, err = gc.upstreamBasedHardReset()
			if err != nil {
				return nil, err
			}
			return wt, nil
		}
		return wt, nil
	}

	if gc.strategy == syngit.TryHardResetOrDie {
		wt, err := gc.upstreamBasedHardReset()
		if err != nil {
			return nil, err
		}
		return wt, nil
	}

	if gc.strategy == syngit.TryMergeCommitOrDie {
		wt, err := gc.upstreamBasedPull()
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

func (gc GitConsistency) upstreamBasedPull() (*git.Worktree, error) {

	return nil, nil
}
