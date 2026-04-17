package helpers

import (
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/test/e2e/syngit/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func BuildDefaultRemoteTarget(fx *utils.Fixture, name, branch string, labels map[string]string) *syngit.RemoteTarget {
	return &syngit.RemoteTarget{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fx.Namespace, Labels: labels},
		Spec: syngit.RemoteTargetSpec{
			UpstreamRepository: fx.RepoURL(),
			TargetRepository:   fx.RepoURL(),
			UpstreamBranch:     "main",
			TargetBranch:       branch,
		},
	}
}

// BuildBranchRemoteTarget builds a RemoteTarget labeled managed-by-syngit
// that targets a specific branch. mergeStrategy is applied when upstream
// and target branches differ.
func BuildBranchRemoteTarget(fx *utils.Fixture, name, upstream, target string,
	mergeStrategy syngit.MergeStrategy) *syngit.RemoteTarget {
	return &syngit.RemoteTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fx.Namespace,
			Labels: map[string]string{
				syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
				syngit.RtLabelKeyBranch:  target,
			},
		},
		Spec: syngit.RemoteTargetSpec{
			UpstreamRepository: fx.RepoURL(),
			TargetRepository:   fx.RepoURL(),
			UpstreamBranch:     upstream,
			TargetBranch:       target,
			MergeStrategy:      mergeStrategy,
		},
	}
}
