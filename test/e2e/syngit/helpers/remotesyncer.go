package helpers

import (
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func BuildDefaultCmRemoteSyncer(name, ns, branch, repoURL string) *syngit.RemoteSyncer {
	return &syngit.RemoteSyncer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				syngit.RtAnnotationKeyOneOrManyBranches: branch,
			},
		},
		Spec: syngit.RemoteSyncerSpec{
			InsecureSkipTlsVerify:       true,
			DefaultBranch:               branch,
			DefaultUnauthorizedUserMode: syngit.Block,
			ExcludedFields:              []string{".metadata.uid"},
			Strategy:                    syngit.CommitApply,
			TargetStrategy:              syngit.OneTarget,
			RemoteRepository:            repoURL,
			ScopedResources: syngit.ScopedResources{
				Rules: []admissionv1.RuleWithOperations{{
					Operations: []admissionv1.OperationType{admissionv1.Create},
					Rule: admissionv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"configmaps"},
					},
				}},
			},
		},
	}
}

func BuildTLSRemoteSyncer(fx *utils.Fixture, name string, caSecretRef *corev1.SecretReference) *syngit.RemoteSyncer {
	rs := &syngit.RemoteSyncer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fx.Namespace,
			Annotations: map[string]string{
				syngit.RtAnnotationKeyOneOrManyBranches: "main",
			},
		},
		Spec: syngit.RemoteSyncerSpec{
			DefaultBranch:               "main",
			DefaultUnauthorizedUserMode: syngit.Block,
			ExcludedFields:              []string{".metadata.uid"},
			Strategy:                    syngit.CommitApply,
			TargetStrategy:              syngit.OneTarget,
			RemoteRepository:            fx.TLSRepoURL(),
			ScopedResources: syngit.ScopedResources{
				Rules: []admissionv1.RuleWithOperations{{
					Operations: []admissionv1.OperationType{admissionv1.Create},
					Rule: admissionv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"configmaps"},
					},
				}},
			},
		},
	}
	if caSecretRef != nil {
		rs.Spec.CABundleSecretRef = *caSecretRef
	}
	return rs
}

func BuildBranchRemoteSyncer(fx *utils.Fixture, name string, annotations map[string]string,
	strategy syngit.TargetStrategy) *syngit.RemoteSyncer {
	return &syngit.RemoteSyncer{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   fx.Namespace,
			Annotations: annotations,
		},
		Spec: syngit.RemoteSyncerSpec{
			InsecureSkipTlsVerify:       true,
			DefaultBranch:               "main",
			DefaultUnauthorizedUserMode: syngit.Block,
			Strategy:                    syngit.CommitApply,
			TargetStrategy:              strategy,
			RemoteRepository:            fx.RepoURL(),
			ScopedResources: syngit.ScopedResources{
				Rules: []admissionv1.RuleWithOperations{{
					Operations: []admissionv1.OperationType{admissionv1.Create},
					Rule: admissionv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"configmaps"},
					},
				}},
			},
		},
	}
}

func BuildRemoteTargetSelectorRS(fx *utils.Fixture, name string, selector *metav1.LabelSelector,
	strategy syngit.TargetStrategy, annotations map[string]string) *syngit.RemoteSyncer {
	return &syngit.RemoteSyncer{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   fx.Namespace,
			Annotations: annotations,
		},
		Spec: syngit.RemoteSyncerSpec{
			InsecureSkipTlsVerify:       true,
			DefaultBranch:               "main",
			DefaultUnauthorizedUserMode: syngit.Block,
			ExcludedFields:              []string{".metadata.uid"},
			Strategy:                    syngit.CommitApply,
			TargetStrategy:              strategy,
			RemoteRepository:            fx.RepoURL(),
			RemoteTargetSelector:        selector,
			ScopedResources: syngit.ScopedResources{
				Rules: []admissionv1.RuleWithOperations{{
					Operations: []admissionv1.OperationType{admissionv1.Create, admissionv1.Delete},
					Rule: admissionv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"configmaps"},
					},
				}},
			},
		},
	}
}

// BuildBranchRS builds a RemoteSyncer that selects a managed RemoteTarget
// by the branch label.
func BuildBranchRS(fx *utils.Fixture, name, upstream, targetBranch string) *syngit.RemoteSyncer {
	return &syngit.RemoteSyncer{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fx.Namespace},
		Spec: syngit.RemoteSyncerSpec{
			InsecureSkipTlsVerify:       true,
			DefaultBranch:               upstream,
			DefaultUnauthorizedUserMode: syngit.Block,
			Strategy:                    syngit.CommitApply,
			TargetStrategy:              syngit.OneTarget,
			RemoteRepository:            fx.RepoURL(),
			RemoteTargetSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
					syngit.RtLabelKeyBranch:  targetBranch,
				},
			},
			ScopedResources: syngit.ScopedResources{
				Rules: []admissionv1.RuleWithOperations{{
					Operations: []admissionv1.OperationType{admissionv1.Create},
					Rule: admissionv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"configmaps"},
					},
				}},
			},
		},
	}
}
