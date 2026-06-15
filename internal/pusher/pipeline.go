package pusher

import (
	"context"
	"fmt"

	"github.com/syngit-org/syngit/internal/mutator"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RunGitPipeline(ctx context.Context, cluster client.Reader, params interceptor.GitPipelineParams) (interceptor.GitPushResponse, error) {
	emptyPaths := make([]string, 0)

	// Get the targeted repository. The lease is held until the pipeline finishes
	// because the repository is mutated through fetch, checkout, commit and push.
	targetRepository, releaseTarget, err := GetTargetRepository(params)
	if err != nil {
		return ResponseBuilder(emptyPaths, "", params.RemoteTarget.Spec.TargetRepository), err
	}
	defer releaseTarget()

	// By default, set the upstream repo the same as the target repo
	// Considering the target branch to be the same as the upstream one
	upstreamRepository := targetRepository

	// If a merge strategy is set and the upstream repository is a different repo
	// than the target, clone it separately. When both point at the same
	// repository URL (only the branch differs) they share a cache key, so we
	// reuse the target lease to avoid acquiring the same per-entry lock twice and
	// deadlocking. This is safe because the merge-strategy paths in GetWorkTree
	// operate solely on the target repository.
	if params.RemoteTarget.Spec.MergeStrategy != "" &&
		params.RemoteTarget.Spec.UpstreamRepository != params.RemoteTarget.Spec.TargetRepository {
		var releaseUpstream func()
		upstreamRepository, releaseUpstream, err = GetUpstreamRepository(params)
		if err != nil {
			return ResponseBuilder(emptyPaths, "", params.RemoteTarget.Spec.TargetRepository), err
		}
		defer releaseUpstream()
	}

	// Pull the worktree
	worktree, needForcePush, err := GetWorkTree(params, targetRepository, upstreamRepository)
	if err != nil {
		return ResponseBuilder(emptyPaths, "", params.RemoteTarget.Spec.TargetRepository),
			syngiterrors.NewGitPipeline(fmt.Sprintf("failed to get worktree: %v", err))
	}

	// Pass over the transformers to generate the final worktree
	var modifiedPaths interceptor.ClaimedPaths
	worktree, modifiedPaths, err = mutator.GenerateFinalWorktree(ctx, cluster, params, worktree)
	if err != nil {
		return ResponseBuilder(emptyPaths, "", params.RemoteTarget.Spec.TargetRepository),
			syngiterrors.NewGitPipeline(fmt.Sprintf("failed to generate the worktree: %v", err))
	}

	// Commit
	commitHash, err := Commit(params, worktree, modifiedPaths, targetRepository)
	if err != nil {
		return ResponseBuilder(GetPathsFromClaimedPaths(modifiedPaths), "", params.RemoteTarget.Spec.TargetRepository),
			syngiterrors.NewGitPipeline(fmt.Sprintf("failed to generate the commit: %v", err))
	}

	// Push
	err = Push(params, targetRepository, needForcePush)
	if err != nil {
		return ResponseBuilder(GetPathsFromClaimedPaths(modifiedPaths), commitHash, params.RemoteTarget.Spec.TargetRepository), err
	}

	return ResponseBuilder(GetPathsFromClaimedPaths(modifiedPaths), commitHash, params.RemoteTarget.Spec.TargetRepository), nil
}
