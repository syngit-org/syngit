package helpers

import (
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildBranchRUB builds a RemoteUserBinding for a specific user referencing
// the given RemoteTargets.
func BuildBranchRUB(fx *utils.Fixture, name, ruName string, targetNames ...string) *syngit.RemoteUserBinding {
	refs := make([]corev1.ObjectReference, 0, len(targetNames))
	for _, t := range targetNames {
		refs = append(refs, corev1.ObjectReference{Name: t})
	}
	return &syngit.RemoteUserBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fx.Namespace},
		Spec: syngit.RemoteUserBindingSpec{
			RemoteUserRefs:   []corev1.ObjectReference{{Name: ruName}},
			RemoteTargetRefs: refs,
			Subject:          rbacv1.Subject{Kind: "User", Name: string(utils.Developer)},
		},
	}
}
