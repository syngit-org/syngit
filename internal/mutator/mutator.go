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
	ModifiedPaths  interceptor.ModifiedPaths
}

// Fill the slice with the mutation providers available in the feature gates.
var mutationsGate = map[features.Feature]Mutator{
	features.HelmValuesMutation: HelmValuesMutation{},
}

var wortreeModifiersGate = map[features.Feature]WorktreeCustomizer{
	features.ResourceFinder: ResourceFinder{},
}

func GenerateFinalWorktree(params interceptor.GitPipelineParams, worktree *git.Worktree) (*git.Worktree, interceptor.ModifiedPaths, error) {
	mutations := &Mutations{}
	for featureGate, mutator := range mutationsGate {
		if features.LoadedFeatureGates.Enabled(featureGate) {
			err := mutator.Mutate(params, mutations)
			if err != nil {
				return worktree, interceptor.NewModifiedPaths(), err
			}
		}
	}

	customWorktree := &CustomWorktree{
		PipelineParams: params,
		Worktree:       worktree,
		ModifiedPaths:  interceptor.NewModifiedPaths(),
	}
	for featureGate, modifier := range wortreeModifiersGate {
		if features.LoadedFeatureGates.Enabled(featureGate) {
			err := modifier.Customize(params, *mutations, customWorktree)
			if err != nil {
				return worktree, interceptor.NewModifiedPaths(), err
			}
		}
	}

	if !customWorktree.ModifiedPaths.IsModified() {
		defaultWorktreeCustomizer := DefaultWorktreeCustomizer{}
		err := defaultWorktreeCustomizer.Customize(params, Mutations{
			params.InterceptedGVR: []byte(params.InterceptedYAML),
		}, customWorktree)
		if err != nil {
			return worktree, interceptor.NewModifiedPaths(), err
		}
	}

	return worktree, customWorktree.ModifiedPaths, nil
}
