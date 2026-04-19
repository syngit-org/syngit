package pusher

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/interceptor"
)

const (
	originRemote   = "origin"
	upstreamRemote = "upstream"
)

func GetWorkTree(
	params interceptor.GitPipelineParams,
	targetRepository, upstreamRepository *git.Repository,
) (*git.Worktree, bool, error) {
	// Same repo & branch between target and upstream
	if targetRepository == upstreamRepository && params.RemoteTarget.Spec.MergeStrategy == "" {
		var err error
		wt, err := targetRepository.Worktree()
		if err != nil {
			return wt, false, err
		}
		return wt, false, nil
	}

	switch params.RemoteTarget.Spec.MergeStrategy {
	case syngit.TryFastForwardOrHardReset:
		wt, err := upstreamBasedPullFastForward(params, targetRepository)
		if err != nil {
			wt, err = upstreamBasedHardReset(params, targetRepository)
			if err != nil {
				return nil, false, err
			}
			return wt, true, nil
		}
		return wt, false, nil
	case syngit.TryHardResetOrDie:
		wt, err := upstreamBasedHardReset(params, targetRepository)
		if err != nil {
			return nil, false, err
		}
		return wt, true, nil
	case syngit.TryFastForwardOrDie:
		wt, err := upstreamBasedPullFastForward(params, targetRepository)
		if err != nil {
			return nil, false, err
		}
		return wt, false, nil
	default:
		return nil, false, fmt.Errorf("wrong target strategy; got %s", params.RemoteTarget.Spec.MergeStrategy)
	}
}

func upstreamBasedHardReset(
	params interceptor.GitPipelineParams,
	targetRepository *git.Repository,
) (*git.Worktree, error) {
	targetBranch := params.RemoteTarget.Spec.TargetBranch
	targetBranchRef := plumbing.NewBranchReferenceName(targetBranch)
	upstreamRemoteRef := plumbing.ReferenceName(fmt.Sprintf("refs/remotes/%s/%s", upstreamRemote, params.RemoteSyncer.Spec.DefaultBranch))

	remErr := fetchUpstream(params, targetRepository)
	if remErr != nil {
		return nil, remErr
	}

	worktree, err := targetRepository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	upstreamLastCommitRef, err := targetRepository.Reference(upstreamRemoteRef, true)
	if err != nil {
		return nil, fmt.Errorf("failed to find remote reference %s: %w", upstreamRemoteRef.String(), err)
	}
	err = worktree.Checkout(&git.CheckoutOptions{
		Hash: upstreamLastCommitRef.Hash(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to checkout upstream commit: %w", err)
	}

	err = targetRepository.Storer.SetReference(plumbing.NewHashReference(targetBranchRef, upstreamLastCommitRef.Hash()))
	if err != nil {
		return nil, fmt.Errorf("failed to create local branch %s: %w", targetBranchRef.String(), err)
	}

	err = checkoutToBranch(targetRepository, worktree, targetBranch)
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

func upstreamBasedPullFastForward(
	params interceptor.GitPipelineParams,
	targetRepository *git.Repository,
) (*git.Worktree, error) {
	upstreamBranch := params.RemoteTarget.Spec.UpstreamBranch
	targetBranch := params.RemoteTarget.Spec.TargetBranch

	remErr := fetchUpstream(params, targetRepository)
	if remErr != nil {
		return nil, remErr
	}

	targetWorktree, err := targetRepository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree for target repository: %w", err)
	}

	err = checkoutToBranch(targetRepository, targetWorktree, targetBranch)
	if err != nil {
		return nil, err
	}

	// STEP 1 : Pull the upstream's commits
	var verboseOutput bytes.Buffer
	upstreamBasedPullOptions := &git.PullOptions{
		RemoteName:    upstreamRemote,
		ReferenceName: plumbing.NewBranchReferenceName(upstreamBranch),
		SingleBranch:  true,
		Auth: &http.BasicAuth{
			Username: params.GitUserInfo.User,
			Password: params.GitUserInfo.Token,
		},
		InsecureSkipTLS: params.RemoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if params.CABundle != nil {
		upstreamBasedPullOptions.CABundle = params.CABundle
	}
	err = targetWorktree.Pull(upstreamBasedPullOptions)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		variables := fmt.Sprintf("\nRemote: %s\nUpstream ref: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
			upstreamRemote,
			plumbing.HEAD,
			targetBranch,
			params.GitUserInfo.User,
			params.GitUserInfo.Token,
		)
		return nil, fmt.Errorf(
			"failed to pull remote: %v\nVerbose output: %s\nVariables: %s",
			err, verboseOutput.String(), variables,
		)
	}

	// STEP 2 : Pull the origin's commits
	originBasedPullOptions := &git.PullOptions{
		RemoteName:    originRemote,
		ReferenceName: plumbing.NewBranchReferenceName(targetBranch),
		SingleBranch:  true,
		Auth: &http.BasicAuth{
			Username: params.GitUserInfo.User,
			Password: params.GitUserInfo.Token,
		},
		InsecureSkipTLS: params.RemoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if params.CABundle != nil {
		originBasedPullOptions.CABundle = params.CABundle
	}
	err = targetWorktree.Pull(originBasedPullOptions)
	if err != nil && err != git.NoErrAlreadyUpToDate && !strings.Contains(err.Error(), "reference not found") {
		variables := fmt.Sprintf("\nRemote: %s\nUpstream ref: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
			upstreamRemote,
			plumbing.HEAD,
			targetBranch,
			params.GitUserInfo.User,
			params.GitUserInfo.Token,
		)
		return nil, fmt.Errorf(
			"failed to pull target remote: %v\nVerbose output: %s\nVariables: %s",
			err, verboseOutput.String(), variables,
		)
	}

	// STEP 3 : Get the reference of the target branch
	targetRef, err := targetRepository.Reference(plumbing.NewBranchReferenceName(targetBranch), true)
	if err != nil {
		return nil, fmt.Errorf("failed to get target branch reference: %w", err)
	}

	err = targetRepository.Storer.SetReference(targetRef)
	if err != nil {
		return nil, fmt.Errorf("failed to set target branch %s: %w", plumbing.NewBranchReferenceName(targetBranch).String(), err)
	}

	// STEP 4 : Get the reference of the upstream branch
	upstreamRef, err := targetRepository.Reference(plumbing.NewBranchReferenceName(upstreamBranch), true)
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream branch reference: %w", err)
	}

	// STEP 5 : Check if the target branch already contains the commit from the upstream branch
	upstreamCommitHash := upstreamRef.Hash()
	contains, err := doesBranchContainsCommit(targetRepository, *targetRef, upstreamCommitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to check if target branch contains commit: %w", err)
	}
	if contains {
		// Target branch already contains the upstream branch commit. Skipping merge.
		return targetWorktree, nil
	}

	// STEP 6 : Try merge (fast-forward) the upstream branch into the target branch
	mergeOptions := &git.MergeOptions{
		Strategy: git.FastForwardMerge,
	}
	mergeErr := targetRepository.Merge(*upstreamRef, *mergeOptions)
	if mergeErr != nil {
		return nil, fmt.Errorf("failed to merge the %s reference in the current branch", upstreamRef.String())
	}

	// Return the updated worktree
	updatedWorktree, err := targetRepository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get updated worktree: %w", err)
	}

	return updatedWorktree, nil
}

func doesBranchContainsCommit(targetRepository *git.Repository, branchRef plumbing.Reference, commitHash plumbing.Hash) (bool, error) {
	branchIter, err := targetRepository.Log(&git.LogOptions{From: branchRef.Hash()})
	if err != nil {
		return false, fmt.Errorf("failed to get branch log: %w", err)
	}
	defer branchIter.Close()

	for {
		commit, err := branchIter.Next()
		if err != nil {
			break
		}
		if commit.Hash == commitHash {
			return true, nil
		}
	}

	return false, nil
}

func checkoutToBranch(targetRepository *git.Repository, worktree *git.Worktree, targetBranch string) error {
	localRef := plumbing.NewBranchReferenceName(targetBranch)
	_, err := targetRepository.Reference(localRef, true)
	if err == plumbing.ErrReferenceNotFound {

		err = worktree.Checkout(&git.CheckoutOptions{
			Branch: localRef,
			Create: true,
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

func fetchUpstream(params interceptor.GitPipelineParams, targetRepository *git.Repository) error {

	upstreamURL := params.RemoteSyncer.Spec.RemoteRepository

	if _, remErr := targetRepository.Remote(upstreamRemote); remErr == git.ErrRemoteNotFound {
		_, err := targetRepository.CreateRemote(&config.RemoteConfig{
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
			Username: params.GitUserInfo.User,
			Password: params.GitUserInfo.Token,
		},
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
			config.RefSpec("+refs/heads/*:refs/remotes/upstream/*"),
		},
		InsecureSkipTLS: params.RemoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if params.CABundle != nil {
		fetchOptions.CABundle = params.CABundle
	}

	err := targetRepository.Fetch(fetchOptions)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		variables := fmt.Sprintf("\nRepository: %s\nUsername: %s\nEmail: %s\n",
			upstreamURL,
			params.GitUserInfo.User,
			params.GitUserInfo.Token,
		)
		return fmt.Errorf("failed to fetch remote: %v\nVerbose output: %s\nVariables: %s", err, verboseOutput.String(), variables)
	}

	return nil
}
