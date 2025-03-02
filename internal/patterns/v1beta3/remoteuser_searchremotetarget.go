package v1beta3

import (
	"context"
	"fmt"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RemoteUserSearchRemoteTargetPattern struct {
	PatternSpecification
	RemoteUser             syngit.RemoteUser
	Username               string
	IsEnabled              bool
	RemoteUserBinding      *syngit.RemoteUserBinding
	remoteTargetsToBeAdded []v1.ObjectReference
}

func (rusp *RemoteUserSearchRemoteTargetPattern) Setup(ctx context.Context) *ErrorPattern {
	if len(rusp.remoteTargetsToBeAdded) > 0 {
		rusp.RemoteUserBinding.Spec.RemoteTargetRefs = append(rusp.RemoteUserBinding.Spec.RemoteTargetRefs, rusp.remoteTargetsToBeAdded...)
		updateErr := updateOrDeleteRemoteUserBinding(ctx, rusp.Client, rusp.RemoteUserBinding.Spec, *rusp.RemoteUserBinding, 2)
		if updateErr != nil {
			return &ErrorPattern{Message: updateErr.Error(), Reason: Errored}
		}
	}
	return nil
}

func (rusp *RemoteUserSearchRemoteTargetPattern) Remove(ctx context.Context) *ErrorPattern {
	// Nothing to remove since it will automatically be done by the RemoteSyncer patterns
	return nil
}

func (rusp *RemoteUserSearchRemoteTargetPattern) Diff(ctx context.Context) *ErrorPattern {
	rusp.remoteTargetsToBeAdded = []v1.ObjectReference{}

	if !rusp.IsEnabled {
		return nil
	}

	// Get the associated RemoteUserBinding
	rubListOps := &client.ListOptions{
		Namespace: rusp.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.K8sUserLabelKey:   rusp.Username,
		}),
	}
	remoteUserBindingList := &syngit.RemoteUserBindingList{}
	listErr := getAssociatedRemoteUserBinding(ctx, rusp.Client, remoteUserBindingList, rubListOps, 5)
	if listErr != nil {
		return &ErrorPattern{Message: listErr.Error(), Reason: Errored}
	}

	if len(remoteUserBindingList.Items) > 1 {
		return &ErrorPattern{Message: fmt.Sprintf("only one RemoteUserBinding for the user %s should be managed by Syngit", rusp.Username), Reason: Denied}
	}
	if len(remoteUserBindingList.Items) == 0 {
		return &ErrorPattern{Message: fmt.Sprintf("a RemoteUserBinding managed by Syngit should exists for the user %s", rusp.Username), Reason: Errored}
	}
	rusp.RemoteUserBinding = &remoteUserBindingList.Items[0]

	existingRemoteTargets, getErr := rusp.getRemoteTargetsThatShouldBeAssociated(ctx)
	if getErr != nil {
		return &ErrorPattern{Message: getErr.Error(), Reason: Errored}
	}

	// Get all the RemoteTargets of bound to the current RemoteUserBinding
	alreadyBoundRemoteTargetsRef := rusp.RemoteUserBinding.Spec.RemoteTargetRefs

	// Fill the slice by the remotetargets that are not yet referenced
	for _, rt := range existingRemoteTargets {
		found := false
		for _, rtRef := range alreadyBoundRemoteTargetsRef {
			if rt.Name == rtRef.Name {
				found = true
				break
			}
		}
		if !found {
			remoteTargetRef := v1.ObjectReference{
				Name: rt.Name,
			}
			rusp.remoteTargetsToBeAdded = append(rusp.remoteTargetsToBeAdded, remoteTargetRef)
		}
	}

	return nil
}

func (rusp *RemoteUserSearchRemoteTargetPattern) getRemoteTargetsThatShouldBeAssociated(ctx context.Context) ([]syngit.RemoteTarget, error) {
	remoteTargets := []syngit.RemoteTarget{}

	// Get all the RemoteTargets that should be bound to all the RemoteUserBindings of the namespace.
	// In other words, all the RemoteTargets that targets a non-unique branch created by
	// the usage of a syngit pattern.
	rtListOps := &client.ListOptions{
		Namespace: rusp.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.RtLabelKeyPattern: syngit.RtLabelValueOneOrManyBranches,
		}),
	}
	ombRemoteTargetList := &syngit.RemoteTargetList{}
	listErr := rusp.Client.List(ctx, ombRemoteTargetList, rtListOps)
	if listErr != nil {
		return nil, listErr
	}
	remoteTargets = append(remoteTargets, ombRemoteTargetList.Items...)

	// Create the RemoteTargets that should be specific to this user
	// TODO

	return remoteTargets, nil
}
