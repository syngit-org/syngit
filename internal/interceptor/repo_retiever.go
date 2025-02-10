package interceptor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
)

const (
	originRemote   = "origin"
	upstreamRemote = "upstream"
)

type RepoRetriever struct {
	gitPusher *GitPusher
}

func (rp RepoRetriever) getRepository(repo string, branch string) (*git.Repository, error) {
	// Clone the repository into memory
	var verboseOutput bytes.Buffer
	cloneOptions := &git.CloneOptions{
		URL:           repo,
		ReferenceName: plumbing.ReferenceName(branch),
		Auth: &http.BasicAuth{
			Username: rp.gitPusher.gitUser,
			Password: rp.gitPusher.gitToken,
		},
		SingleBranch:    true,
		InsecureSkipTLS: rp.gitPusher.remoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if rp.gitPusher.caBundle != nil {
		cloneOptions.CABundle = rp.gitPusher.caBundle
	}
	repository, err := git.Clone(memory.NewStorage(), memfs.New(), cloneOptions)
	if err != nil {
		variables := fmt.Sprintf("\nRepository: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
			repo,
			plumbing.ReferenceName(branch),
			rp.gitPusher.gitUser,
			rp.gitPusher.gitEmail,
		)
		errMsg := fmt.Sprintf("failed to clone repository: %s\nVerbose output: %s\nVariables: %s\n", err.Error(), verboseOutput.String(), variables)
		return nil, errors.New(errMsg)
	}

	return repository, nil
}

func (rp RepoRetriever) GetUpstreamRepository() (*git.Repository, error) {
	return rp.getRepository(rp.gitPusher.remoteTarget.Spec.UpstreamRepository, rp.gitPusher.remoteTarget.Spec.UpstreamBranch)
}

func (rp RepoRetriever) GetTargetRepository() (*git.Repository, error) {
	return rp.getRepository(rp.gitPusher.remoteTarget.Spec.TargetRepository, rp.gitPusher.remoteTarget.Spec.UpstreamBranch)
}

type WorktreeRetriever struct {
	strategy           syngit.MergeStrategy
	targetRepository   *git.Repository
	upstreamRepository *git.Repository
}

func (wr WorktreeRetriever) GetWorkTree(gp GitPusher) (*git.Worktree, bool, error) {

	// Same repo & branch between target and upstream
	if wr.targetRepository == wr.upstreamRepository && wr.strategy == "" {
		var err error
		wt, err := wr.targetRepository.Worktree()
		if err != nil {
			return wt, false, err
		}
		return wt, false, nil
	}

	if wr.strategy == syngit.TryFastForwardOrHardReset {
		wt, err := wr.upstreamBasedPullFastForward(gp)
		if err != nil {
			wt, err = wr.upstreamBasedHardReset(gp)
			if err != nil {
				return nil, false, err
			}
			return wt, true, nil
		}
		return wt, false, nil
	}

	// Different target and upstream
	if wr.strategy == syngit.TryHardResetOrDie {
		wt, err := wr.upstreamBasedHardReset(gp)
		if err != nil {
			return nil, false, err
		}
		return wt, true, nil
	}

	if wr.strategy == syngit.TryFastForwardOrDie {
		wt, err := wr.upstreamBasedPullFastForward(gp)
		if err != nil {
			return nil, false, err
		}
		return wt, false, nil
	}

	return nil, false, fmt.Errorf("wrong target strategy; got %s", wr.strategy)
}

func (wr WorktreeRetriever) upstreamBasedHardReset(gp GitPusher) (*git.Worktree, error) {

	targetBranch := gp.remoteTarget.Spec.TargetBranch
	targetBranchRef := plumbing.NewBranchReferenceName(targetBranch)
	upstreamRemoteRef := plumbing.ReferenceName(fmt.Sprintf("refs/remotes/%s/%s", upstreamRemote, gp.remoteSyncer.Spec.DefaultBranch))

	remErr := wr.fetchUpstream(gp)
	if remErr != nil {
		return nil, remErr
	}

	worktree, err := wr.targetRepository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	upstreamLastCommitRef, err := wr.targetRepository.Reference(upstreamRemoteRef, true)
	if err != nil {
		return nil, fmt.Errorf("failed to find remote reference %s: %w", upstreamRemoteRef.String(), err)
	}
	err = worktree.Checkout(&git.CheckoutOptions{
		Hash: upstreamLastCommitRef.Hash(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to checkout upstream commit: %w", err)
	}

	err = wr.targetRepository.Storer.SetReference(plumbing.NewHashReference(targetBranchRef, upstreamLastCommitRef.Hash()))
	if err != nil {
		return nil, fmt.Errorf("failed to create local branch %s: %w", targetBranchRef.String(), err)
	}

	err = wr.checkoutToBranch(worktree, targetBranch)
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

func (wr WorktreeRetriever) upstreamBasedPullFastForward(gp GitPusher) (*git.Worktree, error) {

	upstreamBranch := gp.remoteTarget.Spec.UpstreamBranch
	targetBranch := gp.remoteTarget.Spec.TargetBranch

	remErr := wr.fetchUpstream(gp)
	if remErr != nil {
		return nil, remErr
	}

	targetWorktree, err := wr.targetRepository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree for target repository: %w", err)
	}

	err = wr.checkoutToBranch(targetWorktree, targetBranch)
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
			Username: gp.gitUser,
			Password: gp.gitToken,
		},
		InsecureSkipTLS: gp.remoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if gp.caBundle != nil {
		upstreamBasedPullOptions.CABundle = gp.caBundle
	}
	err = targetWorktree.Pull(upstreamBasedPullOptions)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		variables := fmt.Sprintf("\nRemote: %s\nUpstream ref: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
			upstreamRemote,
			plumbing.HEAD,
			targetBranch,
			gp.gitUser,
			gp.gitEmail,
		)
		errMsg := fmt.Sprintf("failed to pull remote: %s\nVerbose output: %s\nVariables: %s\n", err.Error(), verboseOutput.String(), variables)
		return nil, errors.New(errMsg)
	}

	// STEP 2 : Pull the origin's commits
	originBasedPullOptions := &git.PullOptions{
		RemoteName:    originRemote,
		ReferenceName: plumbing.NewBranchReferenceName(targetBranch),
		SingleBranch:  true,
		Auth: &http.BasicAuth{
			Username: gp.gitUser,
			Password: gp.gitToken,
		},
		InsecureSkipTLS: gp.remoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if gp.caBundle != nil {
		originBasedPullOptions.CABundle = gp.caBundle
	}
	err = targetWorktree.Pull(originBasedPullOptions)
	if err != nil && err != git.NoErrAlreadyUpToDate && !strings.Contains(err.Error(), "reference not found") {
		variables := fmt.Sprintf("\nRemote: %s\nUpstream ref: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
			upstreamRemote,
			plumbing.HEAD,
			targetBranch,
			gp.gitUser,
			gp.gitEmail,
		)
		errMsg := fmt.Sprintf("failed to pull target remote: %s\nVerbose output: %s\nVariables: %s\n", err.Error(), verboseOutput.String(), variables)
		return nil, errors.New(errMsg)
	}

	// STEP 3 : Get the reference of the target branch
	targetRef, err := wr.targetRepository.Reference(plumbing.NewBranchReferenceName(targetBranch), true)
	if err != nil {
		return nil, fmt.Errorf("failed to get target branch reference: %w", err)
	}

	err = wr.targetRepository.Storer.SetReference(targetRef)
	if err != nil {
		return nil, fmt.Errorf("failed to set target branch %s: %w", plumbing.NewBranchReferenceName(targetBranch).String(), err)
	}

	// STEP 4 : Get the reference of the upstream branch
	upstreamRef, err := wr.targetRepository.Reference(plumbing.NewBranchReferenceName(upstreamBranch), true)
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream branch reference: %w", err)
	}

	// STEP 5 : Check if the target branch already contains the commit from the upstream branch
	upstreamCommitHash := upstreamRef.Hash()
	contains, err := wr.branchContainsCommit(*targetRef, upstreamCommitHash)
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
	mergeErr := wr.targetRepository.Merge(*upstreamRef, *mergeOptions)
	if mergeErr != nil {
		return nil, fmt.Errorf("failed to merge the %s reference in the current branch", upstreamRef.String())
	}

	// Return the updated worktree
	updatedWorktree, err := wr.targetRepository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get updated worktree: %w", err)
	}

	return updatedWorktree, nil
}

func (wr WorktreeRetriever) branchContainsCommit(branchRef plumbing.Reference, commitHash plumbing.Hash) (bool, error) {
	branchIter, err := wr.targetRepository.Log(&git.LogOptions{From: branchRef.Hash()})
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

func (wr WorktreeRetriever) checkoutToBranch(worktree *git.Worktree, targetBranch string) error {
	localRef := plumbing.NewBranchReferenceName(targetBranch)
	_, err := wr.targetRepository.Reference(localRef, true)
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

func (wr WorktreeRetriever) fetchUpstream(gp GitPusher) error {

	upstreamURL := gp.remoteSyncer.Spec.RemoteRepository

	if _, remErr := wr.targetRepository.Remote(upstreamRemote); remErr == git.ErrRemoteNotFound {
		_, err := wr.targetRepository.CreateRemote(&config.RemoteConfig{
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
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
			config.RefSpec("+refs/heads/*:refs/remotes/upstream/*"),
		},
		InsecureSkipTLS: gp.remoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if gp.caBundle != nil {
		fetchOptions.CABundle = gp.caBundle
	}

	err := wr.targetRepository.Fetch(fetchOptions)
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
