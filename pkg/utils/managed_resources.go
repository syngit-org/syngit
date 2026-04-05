package utils

import (
	"context"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpdateOrDeleteManagedRemoteUserBinding updates a managed RemoteUserBinding with
// the given spec, or deletes it if both RemoteUserRefs and RemoteTargetRefs are empty.
func UpdateOrDeleteManagedRemoteUserBinding(
	ctx context.Context,
	k8sClient client.Client,
	spec syngit.RemoteUserBindingSpec,
	remoteUserBinding syngit.RemoteUserBinding,
) error {
	rub := &syngit.RemoteUserBinding{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: remoteUserBinding.Name, Namespace: remoteUserBinding.Namespace}, rub); err != nil { // nolint:lll
		return err
	}

	if len(spec.RemoteUserRefs) == 0 && len(spec.RemoteTargetRefs) == 0 {
		return k8sClient.Delete(ctx, rub)
	}

	rub.Spec = spec
	return k8sClient.Update(ctx, rub)
}

// CreateRemoteTargetAndAssociate creates a RemoteTarget if it doesn't already exist,
// then adds a reference to it in all managed RemoteUserBindings in the same namespace.
func CreateRemoteTargetAndAssociate(ctx context.Context, k8sClient client.Client, remoteTarget *syngit.RemoteTarget) error { // nolint:lll
	if createErr := k8sClient.Create(ctx, remoteTarget); createErr != nil {
		if !apierrors.IsAlreadyExists(createErr) {
			return createErr
		}
	}

	rubs := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		Namespace: remoteTarget.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
		}),
	}
	if err := k8sClient.List(ctx, rubs, listOps); err != nil {
		return err
	}

	for _, rub := range rubs.Items {
		newRtRefs := append(rub.Spec.DeepCopy().RemoteTargetRefs, corev1.ObjectReference{
			Name: remoteTarget.Name,
		})

		spec := rub.Spec
		spec.RemoteTargetRefs = newRtRefs
		if err := UpdateOrDeleteManagedRemoteUserBinding(ctx, k8sClient, spec, rub); err != nil {
			return err
		}
	}

	return nil
}

// RemoveRemoteTargetRefFromManagedRUBs removes a RemoteTarget reference from all
// managed RemoteUserBindings in the given namespace.
func RemoveRemoteTargetRefFromManagedRUBs(ctx context.Context, k8sClient client.Client, namespace, rtName string) error { // nolint:lll
	rubs := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		Namespace: namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
		}),
	}
	if err := k8sClient.List(ctx, rubs, listOps); err != nil {
		return err
	}

	for _, rub := range rubs.Items {
		newRefs := make([]corev1.ObjectReference, 0, len(rub.Spec.RemoteTargetRefs))
		for _, ref := range rub.Spec.RemoteTargetRefs {
			if ref.Name != rtName {
				newRefs = append(newRefs, ref)
			}
		}
		if len(newRefs) == len(rub.Spec.RemoteTargetRefs) {
			continue
		}

		spec := rub.Spec
		spec.RemoteTargetRefs = newRefs
		if err := UpdateOrDeleteManagedRemoteUserBinding(ctx, k8sClient, spec, rub); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
	}

	return nil
}
