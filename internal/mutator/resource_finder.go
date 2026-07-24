package mutator

import (
	"github.com/go-git/go-git/v5"
	"github.com/syngit-org/syngit/internal/walker"
	"github.com/syngit-org/syngit/pkg/interceptor"
)

type ResourceFinder struct{}

// place searches the worktree for the resource matching each artifact and
// replaces its content in place, marking the path as claimed on
// addition/modification/deletion. It is a thin wrapper over ReplaceObject: the
// selector matches by Kubernetes identity and, for non-Kubernetes documents (e.g.
// Helm values), by the ResourceFinderCommentPrefix marker.
func (rf ResourceFinder) place(params interceptor.GitPipelineParams, artifacts ArtifactSet, worktree *git.Worktree) (interceptor.ClaimedPaths, error) {
	claimed := interceptor.NewClaimedPaths()

	scope := params.RemoteTarget.Spec.TargetRepository + "#" + params.RemoteTarget.Spec.UpstreamBranch

	for _, a := range artifacts.Items {
		// Search by the artifact's own logical identity when it carries one.
		// Otherwise fall back to the intercepted object's identity.
		name, namespace := a.Name, a.Namespace
		if name == "" {
			name, namespace = params.InterceptedName, params.RemoteSyncer.Namespace
		}
		sel := walker.ObjectSelector{
			GVR:           a.GVR,
			Name:          name,
			Namespace:     namespace,
			CommentPrefix: ResourceFinderCommentPrefix,
		}

		found, err := walker.ReplaceObject(worktree, scope, sel, a.Content)
		if err != nil {
			return interceptor.NewClaimedPaths(), err
		}
		claimed.AppendClaimedPaths(found)
	}

	return claimed, nil
}
