package helpers

import (
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Builds a ClusterWideRemoteSyncer intercepting the given rules across
// every namespace matching nsSelector, drawing its identities from the
// fixture's own namespace.
//
// A nil nsSelector means every namespace.
func BuildClusterWideRemoteSyncer(
	fx *utils.Fixture,
	name string,
	nsSelector *metav1.LabelSelector,
	rules []admissionv1.RuleWithOperations,
) *syngit.ClusterWideRemoteSyncer {
	return &syngit.ClusterWideRemoteSyncer{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				syngit.RtAnnotationKeyOneOrManyBranches: "main",
			},
		},
		Spec: syngit.ClusterWideRemoteSyncerSpec{
			NamespaceSelector: nsSelector,
			// The fixture namespace holds the RemoteUsers, the managed
			// RemoteUserBindings and the policy-managed RemoteTargets.
			IdentityStoreNamespace: fx.Namespace,
			RemoteSyncerSpec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.BlockDefaultUser,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            fx.RepoURL(),
				ScopedResources: syngit.ScopedResources{
					Rules: rules,
				},
			},
		},
	}
}

// ConfigMapRule scopes CREATE on ConfigMaps.
func ConfigMapRule() admissionv1.RuleWithOperations {
	return admissionv1.RuleWithOperations{
		Operations: []admissionv1.OperationType{admissionv1.Create},
		Rule: admissionv1.Rule{
			APIGroups:   []string{""},
			APIVersions: []string{"v1"},
			Resources:   []string{"configmaps"},
		},
	}
}

// ClusterRoleRule scopes CREATE on ClusterRoles, a cluster-scoped resource.
func ClusterRoleRule() admissionv1.RuleWithOperations {
	return admissionv1.RuleWithOperations{
		Operations: []admissionv1.OperationType{admissionv1.Create},
		Rule: admissionv1.Rule{
			APIGroups:   []string{"rbac.authorization.k8s.io"},
			APIVersions: []string{"v1"},
			Resources:   []string{"clusterroles"},
		},
	}
}
