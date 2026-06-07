package mutator

import (
	"github.com/go-git/go-git/v5"
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

	for _, a := range artifacts.Items {
		sel := ObjectSelector{
			GVR:           a.GVR,
			Name:          params.InterceptedName,
			Namespace:     params.RemoteSyncer.Namespace,
			CommentPrefix: ResourceFinderCommentPrefix,
		}

		found, err := ReplaceObject(worktree, sel, a.Content)
		if err != nil {
			return interceptor.NewClaimedPaths(), err
		}
		claimed.AppendClaimedPaths(found)
	}

	return claimed, nil
}
