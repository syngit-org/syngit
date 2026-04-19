package transformer

import (
	"github.com/go-git/go-git/v5"
	features "github.com/syngit-org/syngit/pkg/feature"
	"github.com/syngit-org/syngit/pkg/interceptor"
)

type Transformer interface {
	Transform(params interceptor.GitPipelineParams, worktree *git.Worktree) (*git.Worktree, interceptor.ModifiedPaths, error)
}

// Fill the slice with the transformers available in the feature gates.
var transformers = map[features.Feature]Transformer{
	features.ResourceFinder: ResourceFinder{},
}

func GenerateFinalWorktree(params interceptor.GitPipelineParams, worktree *git.Worktree) (*git.Worktree, interceptor.ModifiedPaths, error) {
	worktreeModified := false
	modifiedPaths := interceptor.NewModifiedPaths()
	var err error

	for featureGate, transformer := range transformers {
		if features.LoadedFeatureGates.Enabled(featureGate) {
			var paths interceptor.ModifiedPaths

			worktree, paths, err = transformer.Transform(params, worktree)
			if err != nil {
				return worktree, modifiedPaths, err
			}

			if paths.IsModified() {
				worktreeModified = true
				modifiedPaths.AppendModifiedPaths(paths)
			}
		}
	}

	if !worktreeModified {
		defaultTransformer := DefaultTransformer{}
		worktree, modifiedPaths, err = defaultTransformer.Transform(params, worktree)
		if err != nil {
			return worktree, modifiedPaths, err
		}
	}

	return worktree, modifiedPaths, nil
}
