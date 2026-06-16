package pusher

import (
	"bytes"
	"fmt"
	"io"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/interceptor"
)

type GetRepositoryParams struct {
	GitUserInfo  interceptor.GitUserInfo
	RemoteSyncer syngit.RemoteSyncer
	CABundle     []byte
	Repository   string
	Branch       string
}

func (p GetRepositoryParams) cacheKey() string {
	return p.Repository + "#" + p.Branch
}

func (p GetRepositoryParams) basicAuth() *http.BasicAuth {
	return &http.BasicAuth{
		Username: p.GitUserInfo.User,
		Password: p.GitUserInfo.Token,
	}
}

// getRepository returns an in-memory repository for params together with a
// release function that the caller must invoke once it is done mutating the
// repository (clone/refresh -> worktree -> commit -> push).
//
// When the cache is disabled the repository is cloned on every call and the
// release function is a no-op. When the cache is enabled the repository is
// leased: on a miss it is cloned and stored, on a hit it is fetched and hard
// reset back to a fresh-clone-equivalent state before being reused.
func getRepository(params GetRepositoryParams) (*git.Repository, func(), error) {
	if repoCache == nil {
		repository, err := cloneRepository(params)
		return repository, func() {}, err
	}

	lease := acquire(params.cacheKey())

	if lease.repo() == nil {
		// Cache miss: clone and store.
		repository, err := cloneRepository(params)
		if err != nil {
			lease.discard()
			return nil, nil, err
		}
		lease.set(repository)
		return repository, lease.release, nil
	}

	// Cache hit: refresh in place. If refreshing fails for any reason, fall back
	// to a fresh clone and replace the stale cached repository.
	if err := refreshRepository(lease.repo(), params); err != nil {
		repository, cloneErr := cloneRepository(params)
		if cloneErr != nil {
			lease.discard()
			return nil, nil, fmt.Errorf("failed to refresh cached repository: %v; re-clone failed: %w", err, cloneErr)
		}
		lease.set(repository)
		return repository, lease.release, nil
	}

	return lease.repo(), lease.release, nil
}

// cloneRepository clones the repository described by params into memory.
func cloneRepository(params GetRepositoryParams) (*git.Repository, error) {
	var verboseOutput bytes.Buffer
	cloneOptions := &git.CloneOptions{
		URL:             params.Repository,
		ReferenceName:   plumbing.ReferenceName(params.Branch),
		Auth:            params.basicAuth(),
		SingleBranch:    true,
		InsecureSkipTLS: params.RemoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if params.CABundle != nil {
		cloneOptions.CABundle = params.CABundle
	}
	repository, err := git.Clone(memory.NewStorage(), memfs.New(), cloneOptions)
	if err != nil {
		variables := fmt.Sprintf("\nRepository: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
			params.Repository,
			plumbing.ReferenceName(params.Branch),
			params.GitUserInfo.User,
			params.GitUserInfo.Token,
		)
		return nil, fmt.Errorf(
			"failed to clone repository: %v\nVerbose output: %s\nVariables: %s",
			err, verboseOutput.String(), variables,
		)
	}

	return repository, nil
}

// refreshRepository brings a cached repository back to the state a fresh clone
// would produce: it fetches the latest commits from origin, hard resets the
// clone branch to origin/<branch>, cleans untracked files, and prunes any
// leftover branches and the "upstream" remote that a previous pipeline run may
// have created.
func refreshRepository(repository *git.Repository, params GetRepositoryParams) error {
	branch := params.Branch
	branchRef := plumbing.NewBranchReferenceName(branch)
	remoteTrackingRef := plumbing.ReferenceName(fmt.Sprintf("refs/remotes/%s/%s", originRemote, branch))

	var verboseOutput bytes.Buffer
	fetchOptions := &git.FetchOptions{
		RemoteName: originRemote,
		RemoteURL:  params.Repository,
		Auth:       params.basicAuth(),
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", branch, originRemote, branch)),
		},
		InsecureSkipTLS: params.RemoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
		Force:           true,
	}
	if params.CABundle != nil {
		fetchOptions.CABundle = params.CABundle
	}
	if err := repository.Fetch(fetchOptions); err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch origin: %v\nVerbose output: %s", err, verboseOutput.String())
	}

	remoteRef, err := repository.Reference(remoteTrackingRef, true)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", remoteTrackingRef.String(), err)
	}

	if err := repository.Storer.SetReference(plumbing.NewHashReference(branchRef, remoteRef.Hash())); err != nil {
		return fmt.Errorf("failed to reset branch %s: %w", branchRef.String(), err)
	}

	worktree, err := repository.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Branch: branchRef, Force: true}); err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", branch, err)
	}
	if err := worktree.Reset(&git.ResetOptions{Commit: remoteRef.Hash(), Mode: git.HardReset}); err != nil {
		return fmt.Errorf("failed to hard reset: %w", err)
	}
	if err := worktree.Clean(&git.CleanOptions{Dir: true}); err != nil {
		return fmt.Errorf("failed to clean worktree: %w", err)
	}

	if err := pruneStaleState(repository, branchRef); err != nil {
		return err
	}

	return nil
}

// pruneStaleState removes local branches other than keepBranch and the
// "upstream" remote, so the next pipeline run starts from the same state a fresh
// clone would give it.
func pruneStaleState(repository *git.Repository, keepBranch plumbing.ReferenceName) error {
	branches, err := repository.Branches()
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}
	var staleBranches []plumbing.ReferenceName
	err = branches.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name() != keepBranch {
			staleBranches = append(staleBranches, ref.Name())
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to iterate branches: %w", err)
	}
	for _, name := range staleBranches {
		if err := repository.Storer.RemoveReference(name); err != nil {
			return fmt.Errorf("failed to remove stale branch %s: %w", name.String(), err)
		}
	}

	if err := repository.DeleteRemote(upstreamRemote); err != nil && err != git.ErrRemoteNotFound {
		return fmt.Errorf("failed to remove upstream remote: %w", err)
	}

	return nil
}

func GetUpstreamRepository(params interceptor.GitPipelineParams) (*git.Repository, func(), error) {
	return getRepository(GetRepositoryParams{
		RemoteSyncer: params.RemoteSyncer,
		CABundle:     params.CABundle,
		GitUserInfo:  params.GitUserInfo,
		Repository:   params.RemoteTarget.Spec.UpstreamRepository,
		Branch:       params.RemoteTarget.Spec.UpstreamBranch,
	})
}

func GetTargetRepository(params interceptor.GitPipelineParams) (*git.Repository, func(), error) {
	return getRepository(GetRepositoryParams{
		RemoteSyncer: params.RemoteSyncer,
		CABundle:     params.CABundle,
		GitUserInfo:  params.GitUserInfo,
		Repository:   params.RemoteTarget.Spec.TargetRepository,
		Branch:       params.RemoteTarget.Spec.UpstreamBranch,
	})
}
