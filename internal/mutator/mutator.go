package mutator

import (
	"context"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/syngit-org/syngit/internal/walker"
	features "github.com/syngit-org/syngit/pkg/feature"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ResourceFinderCommentPrefix = "syngit.resource-finder/v1: "

// Provider turns an intercepted resource into one or more artifacts to write
// into the git worktree. A Provider may read existing repo state and, when
// available, the live cluster while it produces its artifacts.
type Provider interface {
	// Handles reports whether this provider should run for the intercepted
	// resource described by params. It replaces the in-body GVR guards.
	Handles(params interceptor.GitPipelineParams) bool
	// Render produces artifacts, appending them to out.
	Render(rc RenderContext, out *ArtifactSet) error
}

// RenderContext carries everything a Provider may need to produce its artifacts.
type RenderContext struct {
	Ctx      context.Context
	Params   interceptor.GitPipelineParams
	Worktree *git.Worktree // read existing repo state (overlays, HelmRepository, ...)
	Cluster  client.Reader // optional cluster lookups; nil when unavailable
}

// Artifact is a single file to write into (or delete from) the worktree.
type Artifact struct {
	// GVR is the logical identity of the artifact. When TargetPath is empty it
	// drives the default structured placement and ResourceFinder matching.
	GVR schema.GroupVersionResource
	// Content is the file body. An empty Content marks the artifact as a
	// deletion (see IsDeletion).
	Content []byte
	// TargetPath, when set, is the explicit worktree path for the artifact. Such
	// artifacts are written directly and bypass the placement phase.
	TargetPath string
}

// IsDeletion reports whether the artifact represents a deletion. An artifact
// with no content (the intercepted object was deleted, or a provider emitted
// empty values) is removed from the worktree rather than written.
func (a Artifact) IsDeletion() bool { return len(a.Content) == 0 }

// ArtifactSet accumulates the artifacts produced by the providers.
type ArtifactSet struct {
	Items []Artifact
}

// Add appends an artifact to the set.
func (s *ArtifactSet) Add(a Artifact) { s.Items = append(s.Items, a) }

// providerGate maps feature gates to the providers they enable.
var providerGate = map[features.Feature]Provider{
	features.HelmValuesMutation: HelmValuesMutation{},
	features.FluxHelmRelease:    FluxHelmReleaseProvider{},
}

// GenerateFinalWorktree runs every enabled provider over the intercepted
// resource, then places the produced artifacts into the worktree. Artifacts
// that carry an explicit TargetPath are written directly; the rest flow through
// the reusable placement phase (ResourceFinder, then the default structured
// layout).
func GenerateFinalWorktree(
	ctx context.Context,
	cluster client.Reader,
	params interceptor.GitPipelineParams,
	worktree *git.Worktree,
) (*git.Worktree, interceptor.ClaimedPaths, error) {
	claimedPaths := interceptor.NewClaimedPaths()

	rc := RenderContext{
		Ctx:      ctx,
		Params:   params,
		Worktree: worktree,
		Cluster:  cluster,
	}

	artifacts := &ArtifactSet{}
	for featureGate, provider := range providerGate {
		if !features.LoadedFeatureGates.Enabled(featureGate) {
			continue
		}
		if !provider.Handles(params) {
			continue
		}
		if err := provider.Render(rc, artifacts); err != nil {
			return worktree, interceptor.NewClaimedPaths(), err
		}
	}

	// When no provider produced anything, seed with the original resource so the
	// placement phase has something to act on.
	if len(artifacts.Items) == 0 {
		artifacts.Add(Artifact{
			GVR:     params.InterceptedGVR,
			Content: []byte(params.InterceptedYAML),
		})
	}

	// Artifacts that already carry an explicit path are written directly.
	pathless := ArtifactSet{}
	for _, a := range artifacts.Items {
		if a.TargetPath == "" {
			pathless.Add(a)
			continue
		}
		if err := writeArtifactAtPath(worktree, a, &claimedPaths); err != nil {
			return worktree, interceptor.NewClaimedPaths(), err
		}
	}

	// Path-less artifacts go through the reusable placement phase.
	if len(pathless.Items) > 0 {
		placed, err := placeArtifacts(params, pathless, worktree)
		if err != nil {
			return worktree, interceptor.NewClaimedPaths(), err
		}
		claimedPaths.AppendClaimedPaths(placed)
	}

	return worktree, claimedPaths, nil
}

// placeArtifacts runs the placement phase over path-less artifacts: when the
// ResourceFinder feature and the RemoteSyncer flag are both enabled it tries to
// replace matching resources in existing files; otherwise (or when it claims
// nothing) it falls back to the default structured placement.
func placeArtifacts(params interceptor.GitPipelineParams, artifacts ArtifactSet, worktree *git.Worktree) (interceptor.ClaimedPaths, error) {
	claimed := interceptor.NewClaimedPaths()

	if features.LoadedFeatureGates.Enabled(features.ResourceFinder) && params.RemoteSyncer.Spec.ResourceFinder {
		found, err := (ResourceFinder{}).place(params, artifacts, worktree)
		if err != nil {
			return interceptor.NewClaimedPaths(), err
		}
		claimed.AppendClaimedPaths(found)
	}

	if !claimed.ClaimExists() {
		defaulted, err := (DefaultWorktreeCustomizer{}).place(params, artifacts, worktree)
		if err != nil {
			return interceptor.NewClaimedPaths(), err
		}
		claimed.AppendClaimedPaths(defaulted)
	}

	return claimed, nil
}

// writeArtifactAtPath writes (or deletes) an artifact at its explicit TargetPath
// and records the resulting path in claimed. It is a thin wrapper over
// WriteObjectAtPath: when the file already exists only the document matching the
// artifact's own identity is swapped, so sibling documents are preserved.
func writeArtifactAtPath(worktree *git.Worktree, a Artifact, claimed *interceptor.ClaimedPaths) error {
	placed, err := walker.WriteObjectAtPath(worktree, filepath.Clean(a.TargetPath), walker.SelectorFromDoc(a.Content), a.Content)
	if err != nil {
		return err
	}
	claimed.AppendClaimedPaths(placed)
	return nil
}
