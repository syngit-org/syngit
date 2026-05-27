package utils

import (
	"context"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MutateOrDeleteManagedRemoteUserBinding applies mutate to a freshly-read managed
// RemoteUserBinding and persists the result, retrying on conflict. If after the
// mutation both RemoteUserRefs and RemoteTargetRefs are empty, the RUB is deleted.
//
// The mutation runs against the latest object on every attempt, so concurrent
// reconcilers that each add a different ref merge instead of clobbering each other.
func MutateOrDeleteManagedRemoteUserBinding(
	ctx context.Context,
	k8sClient client.Client,
	name types.NamespacedName,
	mutate func(*syngit.RemoteUserBinding) error,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		rub := &syngit.RemoteUserBinding{}
		if err := k8sClient.Get(ctx, name, rub); err != nil {
			return err
		}

		if err := mutate(rub); err != nil {
			return err
		}

		if len(rub.Spec.RemoteUserRefs) == 0 && len(rub.Spec.RemoteTargetRefs) == 0 {
			return k8sClient.Delete(ctx, rub)
		}
		return k8sClient.Update(ctx, rub)
	})
}

// AddRemoteTargetRef appends a RemoteTarget reference to the RUB if not already present.
func AddRemoteTargetRef(rub *syngit.RemoteUserBinding, rtName string) {
	for _, ref := range rub.Spec.RemoteTargetRefs {
		if ref.Name == rtName {
			return
		}
	}
	rub.Spec.RemoteTargetRefs = append(rub.Spec.RemoteTargetRefs, corev1.ObjectReference{Name: rtName})
}

// RemoveRemoteTargetRef removes a RemoteTarget reference from the RUB.
func RemoveRemoteTargetRef(rub *syngit.RemoteUserBinding, rtName string) {
	newRefs := make([]corev1.ObjectReference, 0, len(rub.Spec.RemoteTargetRefs))
	for _, ref := range rub.Spec.RemoteTargetRefs {
		if ref.Name != rtName {
			newRefs = append(newRefs, ref)
		}
	}
	rub.Spec.RemoteTargetRefs = newRefs
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

	for i := range rubs.Items {
		rub := &rubs.Items[i]
		if err := MutateOrDeleteManagedRemoteUserBinding(ctx, k8sClient,
			types.NamespacedName{Name: rub.Name, Namespace: rub.Namespace},
			func(r *syngit.RemoteUserBinding) error {
				AddRemoteTargetRef(r, remoteTarget.Name)
				return nil
			}); err != nil {
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

	for i := range rubs.Items {
		rub := &rubs.Items[i]
		if err := MutateOrDeleteManagedRemoteUserBinding(ctx, k8sClient,
			types.NamespacedName{Name: rub.Name, Namespace: rub.Namespace},
			func(r *syngit.RemoteUserBinding) error {
				RemoveRemoteTargetRef(r, rtName)
				return nil
			}); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
	}

	return nil
}
