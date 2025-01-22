package interceptor

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
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

const (
	upstreamRemote = "upstream"
)

func getRepository(gp GitPusher, repo string, branch string) (*git.Repository, error) {
	// Clone the repository into memory
	var verboseOutput bytes.Buffer
	cloneOptions := &git.CloneOptions{
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
		cloneOptions.CABundle = gp.caBundle
	}
	repository, err := git.Clone(memory.NewStorage(), memfs.New(), cloneOptions)
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
	return getRepository(gp, gp.remoteTarget.Spec.TargetRepository, gp.remoteTarget.Spec.UpstreamBranch)
}

func (gc GitConsistency) GetWorkTree(gp GitPusher) (*git.Worktree, error) {

	if gc.strategy == syngit.TryPullOrHardReset {
		wt, err := gc.upstreamBasedPull(gp)
		if err != nil {
			wt, err = gc.upstreamBasedHardReset(gp)
			if err != nil {
				return nil, err
			}
			return wt, nil
		}
		return wt, nil
	}

	if gc.strategy == syngit.TryHardResetOrDie {
		wt, err := gc.upstreamBasedHardReset(gp)
		if err != nil {
			return nil, err
		}
		return wt, nil
	}

	if gc.strategy == syngit.TryPullOrDie {
		wt, err := gc.upstreamBasedPull(gp)
		if err != nil {
			return nil, err
		}
		return wt, nil
	}

	return nil, fmt.Errorf("wrong target strategy; got %s", gc.strategy)
}

func (gc GitConsistency) upstreamBasedHardReset(gp GitPusher) (*git.Worktree, error) {

	targetBranch := gp.remoteTarget.Spec.TargetBranch
	targetBranchRef := plumbing.NewBranchReferenceName(targetBranch)
	upstreamRemoteRef := plumbing.ReferenceName(fmt.Sprintf("refs/remotes/%s/%s", upstreamRemote, gp.remoteSyncer.Spec.DefaultBranch))

	remErr := gc.fetchUpstream(gp)
	if remErr != nil {
		return nil, remErr
	}

	// upstreamRef, err := gc.upstreamRepository.Head()
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to get upstream HEAD: %w", err)
	// }

	worktree, err := gc.targetRepository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	upstreamLastCommitRef, err := gc.targetRepository.Reference(upstreamRemoteRef, true)
	if err != nil {
		return nil, fmt.Errorf("failed to find remote reference %s: %w", upstreamRemoteRef.String(), err)
	}
	err = worktree.Checkout(&git.CheckoutOptions{
		Hash: upstreamLastCommitRef.Hash(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to checkout upstream commit: %w", err)
	}

	err = gc.targetRepository.Storer.SetReference(plumbing.NewHashReference(targetBranchRef, upstreamLastCommitRef.Hash()))
	if err != nil {
		return nil, fmt.Errorf("failed to create local branch %s: %w", targetBranchRef.String(), err)
	}

	err = gc.checkoutToBranch(worktree, targetBranch)
	if err != nil {
		return nil, err
	}

	if err := worktree.Reset(&git.ResetOptions{
		Commit: upstreamLastCommitRef.Hash(),
		Mode:   git.HardReset,
	}); err != nil {
		return nil, fmt.Errorf("failed to hard reset: %w", err)
	}

	return worktree, nil
}

func (gc GitConsistency) upstreamBasedPull(gp GitPusher) (*git.Worktree, error) {

	targetBranch := gp.remoteTarget.Spec.TargetBranch
	targetBranchRef := plumbing.NewBranchReferenceName(targetBranch)
	upstreamRemoteRef := plumbing.ReferenceName(fmt.Sprintf("refs/remotes/%s/%s", upstreamRemote, gp.remoteSyncer.Spec.DefaultBranch))

	worktree, err := gc.targetRepository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree for target repository: %w", err)
	}

	remErr := gc.fetchUpstream(gp)
	if remErr != nil {
		return nil, remErr
	}

	// Checkout the upstream default branch in order to pull the diffs
	upstreamLastCommitRef, err := gc.targetRepository.Reference(upstreamRemoteRef, true)
	if err != nil {
		return nil, fmt.Errorf("failed to find remote reference %s: %w", upstreamRemoteRef.String(), err)
	}
	err = worktree.Checkout(&git.CheckoutOptions{
		Hash: upstreamLastCommitRef.Hash(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to checkout upstream commit: %w", err)
	}

	err = gc.targetRepository.Storer.SetReference(plumbing.NewHashReference(targetBranchRef, upstreamLastCommitRef.Hash()))
	if err != nil {
		return nil, fmt.Errorf("failed to create local branch %s: %w", targetBranchRef.String(), err)
	}

	err = gc.checkoutToBranch(worktree, targetBranch)
	if err != nil {
		return nil, err
	}

	var verboseOutput bytes.Buffer
	pullOptions := &git.PullOptions{
		RemoteName:    upstreamRemote,
		ReferenceName: targetBranchRef,
		SingleBranch:  true,
		Auth: &http.BasicAuth{
			Username: gp.gitUser,
			Password: gp.gitToken,
		},
		InsecureSkipTLS: gp.remoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if gp.caBundle != nil {
		pullOptions.CABundle = gp.caBundle
	}
	err = worktree.Pull(pullOptions)
	if err != nil {
		variables := fmt.Sprintf("\nRemote: %s\nUpstream ref: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
			upstreamRemote,
			upstreamLastCommitRef,
			targetBranch,
			gp.gitUser,
			gp.gitEmail,
		)
		errMsg := fmt.Sprintf("failed to pull remote: %s\nVerbose output: %s\nVariables: %s\n", err.Error(), verboseOutput.String(), variables)
		return nil, errors.New(errMsg)
	}

	return worktree, nil
}

func (gc GitConsistency) checkoutToBranch(worktree *git.Worktree, targetBranch string) error {
	localRef := plumbing.NewBranchReferenceName(targetBranch)
	_, err := gc.targetRepository.Reference(localRef, true)
	if err == plumbing.ErrReferenceNotFound {
		// If the branch does not exist locally, create and checkout the branch
		headRef, err := gc.targetRepository.Head()
		if err != nil {
			return fmt.Errorf("failed to get HEAD reference: %w", err)
		}

		err = worktree.Checkout(&git.CheckoutOptions{
			Branch: localRef,
			Create: true,
			Hash:   headRef.Hash(),
		})
		if err != nil {
			return fmt.Errorf("failed to create and checkout branch %s: %w", targetBranch, err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check branch reference: %w", err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: localRef,
	})
	if err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", targetBranch, err)
	}

	return nil
}

func (gc GitConsistency) fetchUpstream(gp GitPusher) error {

	upstreamURL := gp.remoteSyncer.Spec.RemoteRepository
	upstreamBranch := gp.remoteSyncer.Spec.DefaultBranch

	if _, remErr := gc.targetRepository.Remote(upstreamRemote); remErr == git.ErrRemoteNotFound {
		_, err := gc.targetRepository.CreateRemote(&config.RemoteConfig{
			Name: upstreamRemote,
			URLs: []string{upstreamURL},
		})
		if err != nil {
			return fmt.Errorf("failed to create upstream remote: %w", err)
		}
	} else if remErr != nil {
		return fmt.Errorf("failed to get upstream remote: %w", remErr)
	}

	var verboseOutput bytes.Buffer
	fetchOptions := &git.FetchOptions{
		RemoteName: upstreamRemote,
		RemoteURL:  upstreamURL,
		Auth: &http.BasicAuth{
			Username: gp.gitUser,
			Password: gp.gitToken,
		},
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/remotes/upstream/%s", upstreamBranch, upstreamBranch)),
		},
		InsecureSkipTLS: gp.remoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if gp.caBundle != nil {
		fetchOptions.CABundle = gp.caBundle
	}

	err := gc.targetRepository.Fetch(fetchOptions)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		variables := fmt.Sprintf("\nRepository: %s\nUsername: %s\nEmail: %s\n",
			upstreamURL,
			gp.gitUser,
			gp.gitEmail,
		)
		errMsg := fmt.Sprintf("failed to fetch remote: %s\nVerbose output: %s\nVariables: %s\n", err.Error(), verboseOutput.String(), variables)
		return errors.New(errMsg)
	}

	return nil
}
