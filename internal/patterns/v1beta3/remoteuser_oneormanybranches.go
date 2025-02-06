package v1beta3

import (
	"context"
	"fmt"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RemoteUserOneOrManyBranchPattern struct {
	PatternSpecification
	RemoteUser             syngit.RemoteUser
	Username               string
	IsEnabled              bool
	remoteUserBinding      *syngit.RemoteUserBinding
	remoteTargetsToBeAdded []v1.ObjectReference
}

func (rubomp *RemoteUserOneOrManyBranchPattern) Setup(ctx context.Context) *errorPattern {
	if len(rubomp.remoteTargetsToBeAdded) > 0 {
		rubomp.remoteUserBinding.Spec.RemoteTargetRefs = append(rubomp.remoteUserBinding.Spec.RemoteTargetRefs, rubomp.remoteTargetsToBeAdded...)
		updateErr := updateOrDeleteRemoteUserBinding(ctx, rubomp.Client, rubomp.remoteUserBinding.Spec, *rubomp.remoteUserBinding, 2)
		if updateErr != nil {
			return &errorPattern{Message: updateErr.Error(), Reason: Errored}
		}
	}
	return nil
}

func (rubomp *RemoteUserOneOrManyBranchPattern) Remove(ctx context.Context) *errorPattern {
	// Nothing to remove since it will automatically be done by the RemoteSyncer patterns
	return nil
}

func (rubomp *RemoteUserOneOrManyBranchPattern) Diff(ctx context.Context) *errorPattern {
	rubomp.remoteTargetsToBeAdded = []v1.ObjectReference{}

	if !rubomp.IsEnabled {
		return nil
	}

	// Get the associated RemoteUserBinding
	rubListOps := &client.ListOptions{
		Namespace: rubomp.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.K8sUserLabelKey:   rubomp.Username,
		}),
	}
	remoteUserBindingList := &syngit.RemoteUserBindingList{}
	listErr := rubomp.Client.List(ctx, remoteUserBindingList, rubListOps)
	if listErr != nil {
		return &errorPattern{Message: listErr.Error(), Reason: Errored}
	}

	if len(remoteUserBindingList.Items) > 1 {
		return &errorPattern{Message: fmt.Sprintf("only one RemoteUserBinding for the user %s should be managed by Syngit", rubomp.Username), Reason: Denied}
	}
	if len(remoteUserBindingList.Items) == 0 {
		return &errorPattern{Message: fmt.Sprintf("webhook server error: no RemoteUserBinding found for %s", rubomp.Username), Reason: Errored}
	}
	rubomp.remoteUserBinding = &remoteUserBindingList.Items[0]

	// Get all the RemoteTargets that should be bound to all the RemoteUserBindings of the namespace.
	// In other words, all the RemoteTargets that targets a non-unique branch created by
	// the usage of a syngit pattern.
	rtListOps := &client.ListOptions{
		Namespace: rubomp.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.RtLabelPatternKey: syngit.RtLabelOneOrManyBranchesValue,
		}),
	}
	remoteTargetList := &syngit.RemoteTargetList{}
	listErr = rubomp.Client.List(ctx, remoteTargetList, rtListOps)
	if listErr != nil {
		return &errorPattern{Message: listErr.Error(), Reason: Errored}
	}

	// Get all the RemoteTargets of bound to the current RemoteUserBinding
	alreadyBoundRemoteTargetsRef := rubomp.remoteUserBinding.Spec.RemoteTargetRefs

	// Fill the slice by the remotetargets that are not yet referenced
	for _, rt := range remoteTargetList.Items {
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
			rubomp.remoteTargetsToBeAdded = append(rubomp.remoteTargetsToBeAdded, remoteTargetRef)
		}
	}

	return nil
}
