package v1beta4

import (
	"context"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type UserSpecificRemoverPattern struct {
	PatternSpecification
	RemoteSyncer             syngit.RemoteSyncer
	OldUpstreamRepo          string
	OldUpstreamBranch        string
	IsDeleted                bool
	remoteTargetsToBeRemoved []syngit.RemoteTarget
}

func (usrp *UserSpecificRemoverPattern) Remove(ctx context.Context) *ErrorPattern {
	remoteUserBindingList, listErr := usrp.getRemoteUserBindings(ctx)
	if listErr != nil {
		return &ErrorPattern{Message: listErr.Error(), Reason: Errored}
	}

	// Remove association from RemoteUserBinding
	for _, rub := range remoteUserBindingList.Items {

		newRemoteTargetRefs := []v1.ObjectReference{}
		rubSpec := rub.Spec.DeepCopy()

		for _, rtRef := range rub.Spec.RemoteTargetRefs {
			mustBeAdded := true
			for _, rt := range usrp.remoteTargetsToBeRemoved {
				if rtRef.Name == rt.Name {
					mustBeAdded = false
				}
			}
			if mustBeAdded {
				newRemoteTargetRefs = append(newRemoteTargetRefs, rtRef)
			}
		}
		rubSpec.RemoteTargetRefs = newRemoteTargetRefs

		rubUpdateErr := updateOrDeleteRemoteUserBinding(ctx, usrp.Client, *rubSpec, rub, 3)
		if rubUpdateErr != nil {
			return &ErrorPattern{Message: rubUpdateErr.Error(), Reason: Errored}
		}
	}

	for _, rt := range usrp.remoteTargetsToBeRemoved {
		remoteTargetToBeDeleted := &syngit.RemoteTarget{}
		getErr := usrp.Client.Get(ctx, types.NamespacedName{Name: rt.Name, Namespace: rt.Namespace}, remoteTargetToBeDeleted)
		if getErr != nil {
			return &ErrorPattern{Message: getErr.Error(), Reason: Errored}
		}

		delErr := usrp.Client.Delete(ctx, remoteTargetToBeDeleted)
		if delErr != nil {
			return &ErrorPattern{Message: delErr.Error(), Reason: Errored}
		}
	}

	return nil
}

func (usrp *UserSpecificRemoverPattern) Setup(ctx context.Context) *ErrorPattern {
	// Is already setuped by the user specific pattern
	return nil
}

func (usrp *UserSpecificRemoverPattern) Diff(ctx context.Context) *ErrorPattern {
	usrp.remoteTargetsToBeRemoved = []syngit.RemoteTarget{}

	associatedRemoteTargets, rtErr := usrp.getAssociatedNonDependentRemoteTargets(ctx)
	if rtErr != nil {
		return &ErrorPattern{Message: rtErr.Error(), Reason: Errored}
	}

	if usrp.RemoteSyncer.Annotations[syngit.RtAnnotationKeyUserSpecific] == "" || usrp.IsDeleted {
		// The associated must be deleted
		usrp.remoteTargetsToBeRemoved = associatedRemoteTargets
		return nil
	}

	if usrp.RemoteSyncer.Spec.RemoteRepository != usrp.OldUpstreamRepo || usrp.RemoteSyncer.Spec.DefaultBranch != usrp.OldUpstreamBranch {
		for _, rt := range associatedRemoteTargets {
			if rt.Spec.UpstreamRepository == usrp.OldUpstreamRepo || rt.Spec.UpstreamBranch == usrp.OldUpstreamBranch {
				usrp.remoteTargetsToBeRemoved = append(usrp.remoteTargetsToBeRemoved, rt)
			}
		}
	}

	return nil
}

func (usrp *UserSpecificRemoverPattern) getRemoteUserBindings(ctx context.Context) (*syngit.RemoteUserBindingList, error) {
	listOps := &client.ListOptions{
		Namespace: usrp.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
		}),
	}
	remoteUserBindingList := &syngit.RemoteUserBindingList{}
	listErr := usrp.Client.List(ctx, remoteUserBindingList, listOps)
	if listErr != nil {
		return nil, listErr
	}

	return remoteUserBindingList, nil
}

func (usrp *UserSpecificRemoverPattern) getAssociatedNonDependentRemoteTargets(ctx context.Context) ([]syngit.RemoteTarget, error) {
	remoteTargets := []syngit.RemoteTarget{}

	rtListOps := &client.ListOptions{
		Namespace: usrp.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.RtLabelKeyPattern: syngit.RtLabelValueOneUserOneBranch,
		}),
	}
	remoteTargetList := &syngit.RemoteTargetList{}
	listErr := usrp.Client.List(ctx, remoteTargetList, rtListOps)
	if listErr != nil {
		return nil, listErr
	}

	rsyListOps := &client.ListOptions{
		Namespace: usrp.NamespacedName.Namespace,
	}
	remoteSyncerList := &syngit.RemoteSyncerList{}
	listErr = usrp.Client.List(ctx, remoteSyncerList, rsyListOps)
	if listErr != nil {
		return nil, listErr
	}

	for _, rt := range remoteTargetList.Items {
		if (rt.Spec.UpstreamRepository == usrp.RemoteSyncer.Spec.RemoteRepository && rt.Spec.UpstreamBranch == usrp.RemoteSyncer.Spec.DefaultBranch) ||
			(rt.Spec.UpstreamRepository == usrp.OldUpstreamRepo && rt.Spec.UpstreamBranch == usrp.OldUpstreamBranch) {

			for _, rsy := range remoteSyncerList.Items {
				isDependent := false
				if rsy.Name != usrp.RemoteSyncer.Name && rsy.Spec.RemoteRepository == rt.Spec.UpstreamRepository && rsy.Spec.DefaultBranch == rt.Spec.UpstreamBranch {
					isDependent = true
				}
				if !isDependent {
					remoteTargets = append(remoteTargets, rt)
				}
			}

		}
	}

	return remoteTargets, nil
}
