package v1beta3

import (
	"context"
	"slices"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	"github.com/syngit-org/syngit/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RemoteSyncerOneOrManyBranchPattern struct {
	PatternSpecification
	UpstreamRepo             string
	UpstreamBranch           string
	TargetRepository         string
	OldTargetBranches        []string
	NewTargetBranches        []string
	remoteTargetsToBeRemoved []syngit.RemoteTarget
	remoteTargetsToBeSetup   []*syngit.RemoteTarget
	remoteUserBindings       *syngit.RemoteUserBindingList
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) Setup(ctx context.Context) *ErrorPattern {
	for _, remoteTarget := range rsomp.remoteTargetsToBeSetup {
		// Create the RemoteTargets
		createOrUpdateErr := createOrUpdateRemoteTarget(ctx, rsomp.Client, remoteTarget)
		if createOrUpdateErr != nil {
			return &ErrorPattern{Message: createOrUpdateErr.Error(), Reason: Errored}
		}
	}

	// Associate to all the RemoteUserBindings
	associationErr := rsomp.addRemoteUserBindingAssociation(ctx, rsomp.remoteTargetsToBeSetup, *rsomp.remoteUserBindings)
	if associationErr != nil {
		return &ErrorPattern{Message: associationErr.Error(), Reason: Errored}
	}

	return nil
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) addRemoteUserBindingAssociation(ctx context.Context, remoteTargets []*syngit.RemoteTarget, remoteUserBindings syngit.RemoteUserBindingList) error {
	for _, rub := range remoteUserBindings.Items {
		spec := rub.Spec
		newRemoteTargetRefs := spec.RemoteTargetRefs
		for _, rt := range remoteTargets {
			newRemoteTargetRefs = append(newRemoteTargetRefs, v1.ObjectReference{Name: rt.Name})
		}

		spec.RemoteTargetRefs = newRemoteTargetRefs
		err := updateOrDeleteRemoteUserBinding(ctx, rsomp.Client, spec, rub, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) Remove(ctx context.Context) *ErrorPattern {

	for _, rt := range rsomp.remoteTargetsToBeRemoved {
		// Delete RemoteTarget
		delErr := rsomp.Client.Delete(ctx, &rt)
		if delErr != nil {
			return &ErrorPattern{Message: delErr.Error(), Reason: Errored}
		}
	}

	// Remove association from RemoteUserBindings
	associationErr := rsomp.removeRemoteUserBindingAssociation(ctx, rsomp.remoteTargetsToBeRemoved, *rsomp.remoteUserBindings)
	if associationErr != nil {
		return &ErrorPattern{Message: associationErr.Error(), Reason: Errored}
	}

	return nil
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) removeRemoteUserBindingAssociation(ctx context.Context, remoteTargets []syngit.RemoteTarget, remoteUserBindings syngit.RemoteUserBindingList) error {
	for _, rub := range remoteUserBindings.Items {
		spec := rub.Spec
		newRemoteTargetRefs := spec.RemoteTargetRefs

		for _, rt := range remoteTargets {
			for _, associatedRemoteTargetRef := range rub.Spec.RemoteTargetRefs {
				if associatedRemoteTargetRef.Name != rt.Name || associatedRemoteTargetRef.Namespace != rt.Namespace {
					newRemoteTargetRefs = append(newRemoteTargetRefs, associatedRemoteTargetRef)
				}
			}
		}

		spec.RemoteTargetRefs = newRemoteTargetRefs
		err := updateOrDeleteRemoteUserBinding(ctx, rsomp.Client, spec, rub, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) Diff(ctx context.Context) *ErrorPattern {

	listOps := &client.ListOptions{
		Namespace: rsomp.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
		}),
	}

	// Get all RemoteUserBinding of the namespace
	rsomp.remoteUserBindings = &syngit.RemoteUserBindingList{}
	listErr := rsomp.Client.List(ctx, rsomp.remoteUserBindings, listOps)
	if listErr != nil {
		return &ErrorPattern{Message: listErr.Error(), Reason: Errored}
	}

	// Get all RemoteTargets of the namespace
	allRemoteTargets := &syngit.RemoteTargetList{}
	listErr = rsomp.Client.List(ctx, allRemoteTargets, listOps)
	if listErr != nil {
		return &ErrorPattern{Message: listErr.Error(), Reason: Errored}
	}
	// Get only the RemoteTargets that target the same:
	// - upstream repo
	// - upstream branch
	// - target repo
	// - target branches
	// Then, filter the difference between the old and the new branches.
	// Filter out the dependencies with other RemoteSyncers
	// Finally, re-add the new branches
	var filterErr error
	rsomp.remoteTargetsToBeRemoved, filterErr = rsomp.getRemoteTargetsToBeRemoved(ctx, *allRemoteTargets)
	if filterErr != nil {
		return &ErrorPattern{Message: filterErr.Error(), Reason: Errored}
	}

	rsomp.remoteTargetsToBeSetup, filterErr = rsomp.getRemoteTargetsToBeSetup(*allRemoteTargets)
	if filterErr != nil {
		return &ErrorPattern{Message: filterErr.Error(), Reason: Errored}
	}

	return nil
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) getRemoteTargetsToBeSetup(in syngit.RemoteTargetList) ([]*syngit.RemoteTarget, error) {
	branches := []string{}

	// Search for the branches that are not already implemented in a RemoteTarget managed by Syngit
	for _, branch := range rsomp.NewTargetBranches {
		found := false
		for _, rt := range in.Items {
			spec := rt.Spec
			if spec.UpstreamRepository == rsomp.UpstreamRepo && spec.UpstreamBranch == rsomp.UpstreamBranch && spec.TargetRepository == rsomp.TargetRepository {
				if spec.TargetBranch == branch {
					found = true
					break
				}
			}
		}
		if !found {
			branches = append(branches, branch)
		}
	}

	out := []*syngit.RemoteTarget{}
	for _, branch := range branches {
		name, nameErr := utils.RemoteTargetNameConstructor(rsomp.UpstreamRepo, rsomp.UpstreamBranch, rsomp.UpstreamRepo, branch)
		if nameErr != nil {
			return nil, nameErr
		}
		mergeStrategy := syngit.TryFastForwardOrHardReset
		if rsomp.UpstreamBranch == branch {
			mergeStrategy = ""
		}
		remoteTarget := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: rsomp.NamespacedName.Namespace,
				Labels: map[string]string{
					syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
					syngit.RtLabelBranchKey:  branch,
					syngit.RtLabelPatternKey: syngit.RtLabelOneOrManyBranchesValue,
				},
			},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: rsomp.UpstreamRepo,
				UpstreamBranch:     rsomp.UpstreamBranch,
				TargetRepository:   rsomp.TargetRepository,
				TargetBranch:       branch,
				MergeStrategy:      mergeStrategy,
			},
		}
		out = append(out, remoteTarget)
	}

	return out, nil
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) getRemoteTargetsToBeRemoved(ctx context.Context, in syngit.RemoteTargetList) ([]syngit.RemoteTarget, error) {
	out := []syngit.RemoteTarget{}

	// Only filter for the branches that are actually not used anymore by the current RemoteTarget
	diff := slicesDifference(rsomp.OldTargetBranches, rsomp.NewTargetBranches)
	oldBranches := []string{}
	for _, branch := range diff {
		if !slices.Contains(rsomp.NewTargetBranches, branch) {
			oldBranches = append(oldBranches, branch)
		}
	}

	// Search for non-dependent branches
	deletable, depErr := rsomp.getBranchesToBeRemoved(ctx, oldBranches)
	if depErr != nil {
		return nil, depErr
	}

	for _, rt := range in.Items {
		spec := rt.Spec
		if spec.UpstreamRepository == rsomp.UpstreamRepo && spec.UpstreamBranch == rsomp.UpstreamBranch && spec.TargetRepository == rsomp.TargetRepository {
			for _, branch := range deletable {
				if spec.TargetBranch == branch {
					out = append(out, rt)
				}
			}
		}
	}
	return out, nil
}

// Search for automatically created RemoteTargets that are NOT used by any other RemoteSyncer
func (rsomp *RemoteSyncerOneOrManyBranchPattern) getBranchesToBeRemoved(ctx context.Context, branches []string) ([]string, error) {
	out := map[string]bool{}
	for _, branch := range branches {
		out[branch] = true
	}

	remoteSyncers := &syngit.RemoteSyncerList{}
	selector := labels.NewSelector()
	requirement, reqErr := labels.NewRequirement(syngit.RtAnnotationOneOrManyBranchesKey, selection.Exists, nil)
	if reqErr != nil {
		return nil, reqErr
	}
	selector.Add(*requirement)
	listOps := &client.ListOptions{
		Namespace:     rsomp.NamespacedName.Namespace,
		LabelSelector: selector,
	}
	listErr := rsomp.Client.List(ctx, remoteSyncers, listOps)
	if listErr != nil {
		return nil, listErr
	}

	for _, remoteSyncer := range remoteSyncers.Items {
		if remoteSyncer.Name != rsomp.NamespacedName.Name || remoteSyncer.Namespace != rsomp.NamespacedName.Namespace {
			remoteSyncerBranches := utils.GetBranchesFromAnnotation(remoteSyncer.Annotations[syngit.RtAnnotationOneOrManyBranchesKey])
			for _, branch := range branches {
				if slices.Contains(remoteSyncerBranches, branch) {
					out[branch] = false
				}
			}
		}
	}

	branchesToBeRemoved := []string{}
	for branch, toBeRemoved := range out {
		if toBeRemoved {
			branchesToBeRemoved = append(branchesToBeRemoved, branch)
		}
	}

	return branchesToBeRemoved, nil
}
