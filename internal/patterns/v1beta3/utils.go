package v1beta3

import (
	"context"
	"fmt"
	"strings"
	"time"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

func generateName(ctx context.Context, ctrlClient controllerClient.Client, object controllerClient.Object, suffixNumber int) (string, error) {
	oldName := object.GetName()
	newName := object.GetName()
	if suffixNumber > 0 {
		newName = fmt.Sprintf("%s-%d", object.GetName(), suffixNumber)
	}
	webhookNamespacedName := &types.NamespacedName{
		Name:      newName,
		Namespace: object.GetNamespace(),
	}
	getErr := ctrlClient.Get(ctx, *webhookNamespacedName, object)
	if getErr == nil {
		object.SetName(oldName)
		return generateName(ctx, ctrlClient, object, suffixNumber+1)
	} else {
		if strings.Contains(getErr.Error(), "not found") {
			return newName, nil
		}
		return "", getErr
	}
}

// Update the associated RemoteUserBinding with the new spec.
// Delete the associated RemoteUserBinding if the spec is empty.
// The input must be a RemoteUserBinding managed by syngit.
// The retryNumber is used when a conflict happens.
func updateOrDeleteRemoteUserBinding(ctx context.Context, ctrlClient controllerClient.Client, spec syngit.RemoteUserBindingSpec, remoteUserBinding syngit.RemoteUserBinding, retryNumber int) error {
	rub := &syngit.RemoteUserBinding{}
	if err := ctrlClient.Get(ctx, types.NamespacedName{Name: remoteUserBinding.Name, Namespace: remoteUserBinding.Namespace}, rub); err != nil {
		return err
	}

	if len(spec.RemoteUserRefs) == 0 {
		remoteUserBinding.Labels[syngit.ManagedByLabelKey] = ""
		remoteUserBinding.Spec.RemoteTargetRefs = []v1.ObjectReference{}

		if len(spec.RemoteTargetRefs) == 0 {
			delErr := ctrlClient.Delete(ctx, rub)
			if delErr != nil {
				return delErr
			}
			return nil
		}
	}

	rub.Spec = spec
	if err := ctrlClient.Update(ctx, rub); err != nil {
		if retryNumber > 0 {
			return updateOrDeleteRemoteUserBinding(ctx, ctrlClient, spec, remoteUserBinding, retryNumber-1)
		}
		return err
	}
	return nil
}

func createOrUpdateRemoteTarget(ctx context.Context, k8sClient controllerClient.Client, remoteTarget *syngit.RemoteTarget) error {
	if createErr := k8sClient.Create(ctx, remoteTarget); createErr != nil {
		// If it already exists, then we skip this part
		if !strings.Contains(createErr.Error(), "already exists") {
			return createErr
		}
	}

	// Add the association to each RemoteUserBindings
	rubs := &syngit.RemoteUserBindingList{}
	listOps := &controllerClient.ListOptions{
		Namespace: remoteTarget.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
		}),
	}
	listErr := k8sClient.List(ctx, rubs, listOps)
	if listErr != nil {
		return listErr
	}

	for _, rub := range rubs.Items {
		newRtRefs := append(rub.Spec.DeepCopy().RemoteTargetRefs, v1.ObjectReference{
			Name: remoteTarget.Name,
		})

		spec := rub.Spec
		spec.RemoteTargetRefs = newRtRefs
		updateErr := updateOrDeleteRemoteUserBinding(ctx, k8sClient, spec, rub, 2)
		if updateErr != nil {
			return updateErr
		}
	}

	return nil
}

func slicesDifference(slice1 []string, slice2 []string) []string {
	var diff []string

	// Loop two times, first to find slice1 strings not in slice2,
	// second loop to find slice2 strings not in slice1
	for i := 0; i < 2; i++ {
		for _, s1 := range slice1 {
			found := false
			for _, s2 := range slice2 {
				if s1 == s2 {
					found = true
					break
				}
			}
			// String not found. We add it to return slice
			if !found {
				diff = append(diff, s1)
			}
		}
		// Swap the slices, only if it was the first loop
		if i == 0 {
			slice1, slice2 = slice2, slice1
		}
	}

	return diff
}

func getAssociatedRemoteUserBinding(ctx context.Context, k8sClient controllerClient.Client, remoteUserBindingList *syngit.RemoteUserBindingList, listOpts *controllerClient.ListOptions, retryNumber int) error {
	listErr := k8sClient.List(ctx, remoteUserBindingList, listOpts)
	if listErr != nil {
		return listErr
	}

	if len(remoteUserBindingList.Items) == 0 && retryNumber > 0 {
		time.Sleep(500 * time.Millisecond)
		return getAssociatedRemoteUserBinding(ctx, k8sClient, remoteUserBindingList, listOpts, retryNumber-1)
	}
	return nil
}
