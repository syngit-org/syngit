package pusher

import (
	"fmt"

	"github.com/syngit-org/syngit/internal/transformer"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
)

func RunGitPipeline(params syngit.GitPipelineParams) (GitPushResponse, error) {
	emptyPaths := make([]string, 0)

	// Get the targeted repository
	targetRepository, err := GetTargetRepository(params)
	if err != nil {
		return ResponseBuilder(emptyPaths, "", params.RemoteTarget.Spec.TargetRepository), err
	}

	// By default, set the upstream repo the same as the target repo
	// Considering the target branch to be the same as the upstream one
	upstreamRepository := targetRepository

	// If a merge strategy is set, then the target & upstream are different
	if params.RemoteTarget.Spec.MergeStrategy != "" {
		// Different target and upstream
		upstreamRepository, err = GetUpstreamRepository(params)
		if err != nil {
			return ResponseBuilder(emptyPaths, "", params.RemoteTarget.Spec.TargetRepository), err
		}
	}

	// Pull the worktree
	worktree, needForcePush, err := GetWorkTree(params, targetRepository, upstreamRepository)
	if err != nil {
		return ResponseBuilder(emptyPaths, "", params.RemoteTarget.Spec.TargetRepository), fmt.Errorf("failed to get worktree: %v", err)
	}

	// Pass over the transformers to generate the final worktree
	var modifiedPaths syngit.ModifiedPaths
	worktree, modifiedPaths, err = transformer.GenerateFinalWorktree(params, worktree)
	if err != nil {
		return ResponseBuilder(emptyPaths, "", params.RemoteTarget.Spec.TargetRepository), fmt.Errorf("failed to generate the worktree: %v", err)
	}

	// Commit
	commitHash, err := Commit(params, worktree, modifiedPaths, targetRepository)
	if err != nil {
		return ResponseBuilder(GetPathsFromModifiedPaths(modifiedPaths), "", params.RemoteTarget.Spec.TargetRepository), fmt.Errorf("failed to generate the commit: %v", err)
	}

	// Push
	err = Push(params, targetRepository, needForcePush)
	if err != nil {
		return ResponseBuilder(GetPathsFromModifiedPaths(modifiedPaths), commitHash, params.RemoteTarget.Spec.TargetRepository), err
	}

	return ResponseBuilder(GetPathsFromModifiedPaths(modifiedPaths), commitHash, params.RemoteTarget.Spec.TargetRepository), nil
}
