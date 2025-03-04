package v1beta3

import (
	"context"
	"slices"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	"github.com/syngit-org/syngit/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RemoteSyncerOneOrManyBranchPattern struct {
	PatternSpecification
	RemoteSyncer             syngit.RemoteSyncer
	OldUpstreamRepo          string
	OldUpstreamBranch        string
	UpstreamRepo             string
	UpstreamBranch           string
	TargetRepository         string
	OldTargetBranches        []string
	NewTargetBranches        []string
	remoteTargetsToBeRemoved []syngit.RemoteTarget
	remoteTargetsToBeSetup   []*syngit.RemoteTarget
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
	remoteUserBindings, rubErr := rsomp.getRemoteUserBindings(ctx)
	if rubErr != nil {
		return &ErrorPattern{Message: rubErr.Error(), Reason: Errored}
	}
	associationErr := rsomp.addRemoteUserBindingAssociation(ctx, rsomp.remoteTargetsToBeSetup, *remoteUserBindings)
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

	// Remove association from RemoteUserBindings
	remoteUserBindings, rubErr := rsomp.getRemoteUserBindings(ctx)
	if rubErr != nil {
		return &ErrorPattern{Message: rubErr.Error(), Reason: Errored}
	}
	associationErr := rsomp.removeRemoteUserBindingAssociation(ctx, rsomp.remoteTargetsToBeRemoved, *remoteUserBindings)
	if associationErr != nil {
		return &ErrorPattern{Message: associationErr.Error(), Reason: Errored}
	}

	for _, rt := range rsomp.remoteTargetsToBeRemoved {
		// Delete RemoteTarget
		delErr := rsomp.Client.Delete(ctx, &rt)
		if delErr != nil {
			return &ErrorPattern{Message: delErr.Error(), Reason: Errored}
		}
	}

	return nil
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) removeRemoteUserBindingAssociation(ctx context.Context, remoteTargets []syngit.RemoteTarget, remoteUserBindings syngit.RemoteUserBindingList) error {
	for _, rub := range remoteUserBindings.Items {
		spec := rub.Spec.DeepCopy()
		newRemoteTargetRefs := []v1.ObjectReference{}

		for _, associatedRemoteTargetRef := range rub.Spec.RemoteTargetRefs {
			mustBeAdded := true
			for _, rt := range remoteTargets {
				if associatedRemoteTargetRef.Name == rt.Name {
					mustBeAdded = false
				}
			}
			if mustBeAdded {
				newRemoteTargetRefs = append(newRemoteTargetRefs, associatedRemoteTargetRef)
			}
		}

		spec.RemoteTargetRefs = newRemoteTargetRefs
		err := updateOrDeleteRemoteUserBinding(ctx, rsomp.Client, *spec, rub, 2)
		if err != nil {
			return err
		}
	}
	return nil
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) getRemoteUserBindings(ctx context.Context) (*syngit.RemoteUserBindingList, error) {

	rubListOps := &client.ListOptions{
		Namespace: rsomp.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
		}),
	}
	// Get all RemoteUserBinding of the namespace
	remoteUserBindings := &syngit.RemoteUserBindingList{}
	listErr := rsomp.Client.List(ctx, remoteUserBindings, rubListOps)
	if listErr != nil {
		return nil, listErr
	}

	return remoteUserBindings, nil
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) Diff(ctx context.Context) *ErrorPattern {
	rtListOps := &client.ListOptions{
		Namespace: rsomp.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.RtLabelKeyPattern: syngit.RtLabelValueOneOrManyBranches,
		}),
	}
	// Get all RemoteTargets of the namespace
	allRemoteTargets := &syngit.RemoteTargetList{}
	listErr := rsomp.Client.List(ctx, allRemoteTargets, rtListOps)
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
	branches := rsomp.getBranchesToBeSetup(in)

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
					syngit.RtLabelKeyBranch:  branch,
					syngit.RtLabelKeyPattern: syngit.RtLabelValueOneOrManyBranches,
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

func (rsomp *RemoteSyncerOneOrManyBranchPattern) getBranchesToBeSetup(in syngit.RemoteTargetList) []string {
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

	if rsomp.UpstreamRepo != rsomp.OldUpstreamRepo || rsomp.UpstreamBranch != rsomp.OldUpstreamBranch {
		branches = utils.GetBranchesFromAnnotation(rsomp.RemoteSyncer.Annotations[syngit.RtAnnotationKeyOneOrManyBranches])
	}

	return branches
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

	// Get RemoteSyncers that manage RemoteTargets using this pattern
	remoteSyncers, listErr := rsomp.getRemoteSyncersManagedByThisPattern(ctx)
	if listErr != nil {
		return nil, listErr
	}

	// Search for non-dependent branches
	deletable := rsomp.getBranchesToBeRemoved(oldBranches, remoteSyncers)

	for _, rt := range in.Items {
		spec := rt.Spec
		if (rsomp.UpstreamRepo != rsomp.OldUpstreamRepo && spec.UpstreamRepository == rsomp.OldUpstreamRepo) ||
			(rsomp.UpstreamBranch != rsomp.OldUpstreamBranch && spec.UpstreamBranch == rsomp.OldUpstreamBranch) {
			if rsomp.isRemoteTargetUnused(rt, rsomp.OldUpstreamRepo, rsomp.OldUpstreamBranch, remoteSyncers) {
				out = append(out, rt)
			}
		}
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

// Get RemoteSyncers that manage RemoteTargets using this pattern
func (rsomp *RemoteSyncerOneOrManyBranchPattern) getRemoteSyncersManagedByThisPattern(ctx context.Context) ([]syngit.RemoteSyncer, error) {
	remoteSyncers := []syngit.RemoteSyncer{}

	remoteSyncerList := &syngit.RemoteSyncerList{}
	listOps := &client.ListOptions{
		Namespace: rsomp.NamespacedName.Namespace,
	}
	listErr := rsomp.Client.List(ctx, remoteSyncerList, listOps)
	if listErr != nil {
		return nil, listErr
	}

	for _, rsy := range remoteSyncerList.Items {
		if rsy.Annotations[syngit.RtAnnotationKeyOneOrManyBranches] != "" {
			remoteSyncers = append(remoteSyncers, rsy)
		}
	}

	return remoteSyncers, nil
}

// Check if the RemoteTarget is unused by searching is a managed RemoteSyncer use it
func (rsomp *RemoteSyncerOneOrManyBranchPattern) isRemoteTargetUnused(remoteTarget syngit.RemoteTarget, oldRepository string, oldBranch string, remoteSyncers []syngit.RemoteSyncer) bool {
	if remoteTarget.Labels[syngit.RtLabelKeyPattern] != string(syngit.RtLabelValueOneOrManyBranches) {
		// This case is not managed by this pattern
		return false
	}

	for _, rsy := range remoteSyncers {
		if rsy.Name != rsomp.RemoteSyncer.Name && (rsy.Spec.RemoteRepository == oldRepository || rsy.Spec.DefaultBranch == oldBranch) {
			return false
		}
	}

	return true
}

// Search for automatically created RemoteTargets that are NOT used by any other RemoteSyncer (filtering by branches)
func (rsomp *RemoteSyncerOneOrManyBranchPattern) getBranchesToBeRemoved(branches []string, remoteSyncers []syngit.RemoteSyncer) []string {
	out := map[string]bool{}
	for _, branch := range branches {
		out[branch] = true
	}

	for _, remoteSyncer := range remoteSyncers {
		if remoteSyncer.Name != rsomp.NamespacedName.Name || remoteSyncer.Namespace != rsomp.NamespacedName.Namespace {
			remoteSyncerBranches := utils.GetBranchesFromAnnotation(remoteSyncer.Annotations[syngit.RtAnnotationKeyOneOrManyBranches])
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

	return branchesToBeRemoved
}
