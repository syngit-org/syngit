package v1beta3

import (
	"context"
	"fmt"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RemoteUserAssociationPattern struct {
	PatternSpecification
	RemoteUser            syngit.RemoteUser
	IsEnabled             bool
	remoteUserBindingList *syngit.RemoteUserBindingList
}

func (ruap *RemoteUserAssociationPattern) RemoveExistingOnes(ctx context.Context) error {
	if len(ruap.remoteUserBindingList.Items) == 1 {
		// Select the first one because there must not be more than one
		rub := ruap.remoteUserBindingList.Items[0]
		// Remove the RemoteUser from the associated RemoteUserBinding
		return ruap.removeRuFromRub(ctx, &rub)
	}
	return nil
}

func (ruap *RemoteUserAssociationPattern) Trigger(ctx context.Context) *errorPattern {

	name := syngit.RubPrefix + ruap.Username

	ruap.remoteUserBindingList = &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.K8sUserLabelKey:   ruap.Username,
		}),
		Namespace: ruap.NamespacedName.Namespace,
	}
	rubErr := ruap.Client.List(ctx, ruap.remoteUserBindingList, listOps)
	if rubErr != nil {
		return &errorPattern{Message: rubErr.Error(), Reason: Errored}
	}

	if len(ruap.remoteUserBindingList.Items) > 1 {
		return &errorPattern{Message: fmt.Sprintf("only one RemoteUserBinding for the user %s should be managed by Syngit", ruap.Username), Reason: Denied}
	}

	objRef := corev1.ObjectReference{Name: ruap.RemoteUser.Name}

	// Check if the association already exists before deleting it
	isAlreadyDefined, rubFoundName, definedErr := ruap.isAlreadyReferenced(ctx, ruap.RemoteUser.Name, ruap.RemoteUser.Namespace)
	if definedErr != nil {
		return &errorPattern{Message: definedErr.Error(), Reason: Errored}
	}

	if len(ruap.remoteUserBindingList.Items) <= 0 {
		// CREATE

		if !ruap.IsEnabled {
			// The pattern is not enabled and no RemoteUserBinding is associated
			return nil
		}

		rub := &syngit.RemoteUserBinding{}
		rub.SetName(name)
		rub.SetNamespace(ruap.RemoteUser.Namespace)

		// Geneate the RUB object with the right name
		rubName, generateErr := generateName(ctx, ruap.Client, rub, 0)
		if generateErr != nil {
			return &errorPattern{Message: generateErr.Error(), Reason: Errored}
		}
		if isAlreadyDefined && name != rubFoundName {
			return &errorPattern{Message: fmt.Sprintf("the RemoteUser is already bound in the RemoteUserBinding %s", rubFoundName), Reason: Denied}
		}
		rub.SetName(rubName)

		// Set the labels
		rub.Labels = map[string]string{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.K8sUserLabelKey:   ruap.Username,
		}

		subject := &rbacv1.Subject{
			Kind: "User",
			Name: ruap.Username,
		}
		rub.Spec.Subject = *subject

		remoteRefs := make([]corev1.ObjectReference, 0)
		remoteRefs = append(remoteRefs, objRef)
		rub.Spec.RemoteUserRefs = remoteRefs

		// In any case, remove the association to add it after
		removeErr := ruap.RemoveExistingOnes(ctx)
		if removeErr != nil {
			return &errorPattern{Message: removeErr.Error(), Reason: Errored}
		}

		createErr := ruap.Client.Create(ctx, rub)
		if createErr != nil {
			return &errorPattern{Message: createErr.Error(), Reason: Errored}
		}

	} else {
		// UPDATE or DELETE
		// The RemoteUserBinding already exists

		rub := ruap.remoteUserBindingList.Items[0]
		name = rub.Name

		if isAlreadyDefined && name != rubFoundName {
			return &errorPattern{Message: fmt.Sprintf("the RemoteUser is already bound in the RemoteUserBinding %s", rubFoundName), Reason: Denied}
		}

		if !ruap.IsEnabled {
			// The pattern is not enabled and a RemoteUserBinding is associated
			// So it is a delete operation
			removeErr := ruap.RemoveExistingOnes(ctx)
			if removeErr != nil {
				return &errorPattern{Message: removeErr.Error(), Reason: Errored}
			}
		}

		dontAppend := false
		remoteRefs := rub.DeepCopy().Spec.RemoteUserRefs
		for _, ruRef := range remoteRefs {
			if ruRef.Name == ruap.RemoteUser.Name {
				dontAppend = true
			}
		}
		if !dontAppend {
			remoteRefs = append(remoteRefs, objRef)
		}
		rub.Spec.RemoteUserRefs = remoteRefs

		// In any case, remove the association to update it after
		removeErr := ruap.RemoveExistingOnes(ctx)
		if removeErr != nil {
			return &errorPattern{Message: removeErr.Error(), Reason: Errored}
		}

		updateErr := ruap.Client.Update(ctx, &rub)
		if updateErr != nil {
			return &errorPattern{Message: updateErr.Error(), Reason: Errored}
		}

	}

	return nil
}

func (ruap *RemoteUserAssociationPattern) isAlreadyReferenced(ctx context.Context, ruName string, ruNamespace string) (bool, string, error) {
	rubs := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		Namespace: ruNamespace,
	}
	rubErr := ruap.Client.List(ctx, rubs, listOps)
	if rubErr != nil {
		return false, "", rubErr
	}

	for _, rub := range rubs.Items {
		for _, ru := range rub.Spec.RemoteUserRefs {
			if ru.Name == ruName {
				if ru.Namespace == "" || (ru.Namespace == ruNamespace) {
					return true, rub.Name, nil
				}
			}
		}
	}

	return false, "", nil
}

func (ruap *RemoteUserAssociationPattern) removeRuFromRub(ctx context.Context, rub *syngit.RemoteUserBinding) error {
	remoteRefs := rub.Spec.DeepCopy().RemoteUserRefs
	newRemoteRefs := []corev1.ObjectReference{}
	for _, rm := range remoteRefs {
		if rm.Name != ruap.RemoteUser.Name {
			newRemoteRefs = append(newRemoteRefs, rm)
		}
	}

	if len(newRemoteRefs) != 0 {

		rub.Spec.RemoteUserRefs = newRemoteRefs
		deleteErr := ruap.Client.Update(ctx, rub)
		if deleteErr != nil {
			return &errorPattern{Message: deleteErr.Error(), Reason: Errored}
		}

	} else {

		deleteErr := ruap.Client.Delete(ctx, rub)
		if deleteErr != nil {
			return &errorPattern{Message: deleteErr.Error(), Reason: Errored}
		}
	}

	return nil
}
