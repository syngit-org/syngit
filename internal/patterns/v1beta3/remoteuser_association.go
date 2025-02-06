package v1beta3

import (
	"context"
	"fmt"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RemoteUserAssociationPattern struct {
	PatternSpecification
	RemoteUser                  syngit.RemoteUser
	IsEnabled                   bool
	Username                    string
	associatedRemoteUserBinding *syngit.RemoteUserBinding
	hasToBeRemoved              bool
	hasToBeSetup                bool
}

func (ruap *RemoteUserAssociationPattern) Remove(ctx context.Context) *errorPattern {
	if ruap.hasToBeRemoved {
		// Select the first one because there must not be more than one
		rub := ruap.associatedRemoteUserBinding
		// Remove the RemoteUser from the associated RemoteUserBinding
		if err := ruap.removeRuFromRub(ctx, rub); err != nil {
			return &errorPattern{Message: err.Error(), Reason: Errored}
		}
	}
	return nil
}

func (ruap *RemoteUserAssociationPattern) Setup(ctx context.Context) *errorPattern {
	if ruap.hasToBeSetup {
		updateErr := updateOrDeleteRemoteUserBinding(ctx, ruap.Client, ruap.associatedRemoteUserBinding.Spec, *ruap.associatedRemoteUserBinding, 2)
		if updateErr != nil {
			if !strings.Contains(updateErr.Error(), "not found") {
				return &errorPattern{Message: updateErr.Error(), Reason: Errored}
			}
			createErr := ruap.Client.Create(ctx, ruap.associatedRemoteUserBinding)
			if createErr != nil {
				return &errorPattern{Message: createErr.Error(), Reason: Errored}
			}
		}
	}

	return nil
}

func (ruap *RemoteUserAssociationPattern) Diff(ctx context.Context) *errorPattern {

	ruap.hasToBeRemoved = false
	ruap.hasToBeSetup = false

	// Check if the association is already done
	isAlreadyDefined, existingRemoteUserBindingName, diffErr := ruap.isAlreadyReferenced(ctx, ruap.RemoteUser.Name, ruap.NamespacedName.Namespace)
	if diffErr != nil {
		return &errorPattern{Message: diffErr.Error(), Reason: Errored}
	}

	name := syngit.RubPrefix + ruap.Username

	// List all the RemoteUserBindings that are associated to this user and managed by Syngit.
	remoteUserBindingList := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.K8sUserLabelKey:   ruap.Username,
		}),
		Namespace: ruap.NamespacedName.Namespace,
	}
	rubErr := ruap.Client.List(ctx, remoteUserBindingList, listOps)
	if rubErr != nil {
		return &errorPattern{Message: rubErr.Error(), Reason: Errored}
	}

	if len(remoteUserBindingList.Items) > 1 {
		return &errorPattern{Message: fmt.Sprintf("only one RemoteUserBinding for the user %s should be managed by Syngit", ruap.Username), Reason: Denied}
	}

	objRef := corev1.ObjectReference{Name: ruap.RemoteUser.Name}

	if len(remoteUserBindingList.Items) <= 0 {

		if !ruap.IsEnabled {
			// The pattern is not enabled and no RemoteUserBinding is associated
			return nil
		}

		if isAlreadyDefined && name != existingRemoteUserBindingName {
			return &errorPattern{Message: fmt.Sprintf("the RemoteUser is already bound in the RemoteUserBinding %s", existingRemoteUserBindingName), Reason: Denied}
		}

		// CREATE the RemoteUserBinding

		rub := &syngit.RemoteUserBinding{}
		rub.SetName(name)
		rub.SetNamespace(ruap.RemoteUser.Namespace)

		// Geneate the RUB object with the right name
		rubName, generateErr := generateName(ctx, ruap.Client, rub.DeepCopy(), 0)
		if generateErr != nil {
			return &errorPattern{Message: generateErr.Error(), Reason: Errored}
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

		ruap.associatedRemoteUserBinding = rub
		if ruap.IsEnabled {
			ruap.hasToBeSetup = true
		}

	} else {
		// UPDATE or DELETE
		// The RemoteUserBinding already exists

		rub := remoteUserBindingList.Items[0]
		name = rub.Name

		if isAlreadyDefined && name != existingRemoteUserBindingName {
			return &errorPattern{Message: fmt.Sprintf("the RemoteUser is already bound in the RemoteUserBinding %s", existingRemoteUserBindingName), Reason: Denied}
		}

		remoteUserBinding := &syngit.RemoteUserBinding{}

		if getErr := ruap.Client.Get(ctx, types.NamespacedName{Name: rub.Name, Namespace: rub.Namespace}, remoteUserBinding); getErr != nil {
			return &errorPattern{Message: getErr.Error(), Reason: Errored}
		}

		ruap.associatedRemoteUserBinding = remoteUserBinding

		if !ruap.IsEnabled {
			// The pattern is not enabled and a RemoteUserBinding is associated
			// So it is a delete operation
			ruap.hasToBeRemoved = true
		}

		// Is the current RemoteUser already associated?
		for _, remoteUserRef := range remoteUserBinding.Spec.RemoteUserRefs {
			if remoteUserRef.Name == ruap.RemoteUser.Name {
				return nil
			}
		}

		remoteUserBinding.Spec.RemoteUserRefs = append(remoteUserBinding.Spec.RemoteUserRefs, objRef)

		if ruap.IsEnabled {
			ruap.hasToBeSetup = true
		}
		ruap.associatedRemoteUserBinding = remoteUserBinding

	}

	return nil
}

// Search for a RemoteUserBinding that reference this k8s user.
// It can already be referenced by ANOTHER user.
func (ruap *RemoteUserAssociationPattern) isAlreadyReferenced(ctx context.Context, ruName string, ruNamespace string) (bool, string, error) {
	rubs := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
		}),
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
	rub.Spec.RemoteUserRefs = newRemoteRefs

	updateOrDeleteRemoteUserBinding(ctx, ruap.Client, rub.Spec, *rub, 0)

	return nil
}
