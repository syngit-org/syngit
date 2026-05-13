package mutator

import (
	"github.com/go-git/go-git/v5"
	features "github.com/syngit-org/syngit/pkg/feature"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const ResourceFinderCommentPrefix = "syngit.resource-finder/v1: "

type Mutator interface {
	Mutate(params interceptor.GitPipelineParams, mutations *Mutations) error
}

type Mutations map[schema.GroupVersionResource][]byte

func (mutations Mutations) AddMutation(gvr schema.GroupVersionResource, content []byte) {
	mutations[gvr] = content
}

type WorktreeCustomizer interface {
	Customize(params interceptor.GitPipelineParams, mutations Mutations, customWorktree *CustomWorktree) error
}

type CustomWorktree struct {
	PipelineParams interceptor.GitPipelineParams
	Worktree       *git.Worktree
	ClaimedPaths   interceptor.ClaimedPaths
}

// Fill the slice with the mutation providers available in the feature gates.
var mutationsGate = map[features.Feature]Mutator{
	features.HelmValuesMutation: HelmValuesMutation{},
}

var wortreeModifiersGate = map[features.Feature]WorktreeCustomizer{
	features.ResourceFinder: ResourceFinder{},
}

func GenerateFinalWorktree(params interceptor.GitPipelineParams, worktree *git.Worktree) (*git.Worktree, interceptor.ClaimedPaths, error) {
	mutations := &Mutations{}
	for featureGate, mutator := range mutationsGate {
		if features.LoadedFeatureGates.Enabled(featureGate) {
			err := mutator.Mutate(params, mutations)
			if err != nil {
				return worktree, interceptor.NewClaimedPaths(), err
			}
		}
	}

	// When no Mutator produced anything, seed mutations with the original
	// resource so downstream WorktreeCustomizers (ResourceFinder, fallback)
	// have something to act on.
	if len(*mutations) == 0 {
		mutations.AddMutation(params.InterceptedGVR, []byte(params.InterceptedYAML))
	}

	customWorktree := &CustomWorktree{
		PipelineParams: params,
		Worktree:       worktree,
		ClaimedPaths:   interceptor.NewClaimedPaths(),
	}
	for featureGate, modifier := range wortreeModifiersGate {
		if features.LoadedFeatureGates.Enabled(featureGate) {
			err := modifier.Customize(params, *mutations, customWorktree)
			if err != nil {
				return worktree, interceptor.NewClaimedPaths(), err
			}
		}
	}

	if !customWorktree.ClaimedPaths.ClaimExists() {
		err := DefaultWorktreeCustomizer{}.Customize(params, *mutations, customWorktree)
		if err != nil {
			return worktree, interceptor.NewClaimedPaths(), err
		}
	}

	return worktree, customWorktree.ClaimedPaths, nil
}
