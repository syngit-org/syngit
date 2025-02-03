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
	UpstreamRepo          string
	UpstreamBranch        string
	TargetRepository      string
	GarbageTargetBranches []string
	TargetBranches        []string
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) Trigger(ctx context.Context) *errorPattern {

	removeErr := rsomp.RemoveExistingOnes(ctx)
	if removeErr != nil {
		return &errorPattern{Message: removeErr.Error(), Reason: Errored}
	}

	// Create the RemoteTargets
	for _, branch := range rsomp.TargetBranches {
		name, nameErr := utils.RemoteTargetNameConstructor(rsomp.UpstreamRepo, rsomp.UpstreamBranch, branch)
		if nameErr != nil {
			return &errorPattern{Message: nameErr.Error(), Reason: Errored}
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
		createOrUpdateErr := createOrUpdateRemoteTarget(ctx, rsomp.Client, remoteTarget)
		if createOrUpdateErr != nil {
			return &errorPattern{Message: createOrUpdateErr.Error(), Reason: Errored}
		}
	}

	return nil
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) RemoveExistingOnes(ctx context.Context) error {
	listOps := &client.ListOptions{
		Namespace: rsomp.NamespacedName.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
		}),
	}

	// Get all RemoteUserBinding of the namespace
	allRemoteUserBindings := &syngit.RemoteUserBindingList{}
	listErr := rsomp.Client.List(ctx, allRemoteUserBindings, listOps)
	if listErr != nil {
		return listErr
	}

	// Get all RemoteTargets of the namespace
	allRemoteTargets := &syngit.RemoteTargetList{}
	listErr = rsomp.Client.List(ctx, allRemoteTargets, listOps)
	if listErr != nil {
		return listErr
	}
	// Get only the RemoteTargets that target the same:
	// - upstream repo
	// - upstream branch
	// - target repo
	// - target branches
	// Then, filter the difference between the old and the new branches.
	// Filter out the dependencies with other RemoteSyncers
	// Finally, re-add the new branches
	filteredRemoteTargets, filterErr := rsomp.filterRemoteTargets(ctx, *allRemoteTargets)
	if filterErr != nil {
		return filterErr
	}

	for _, rt := range filteredRemoteTargets {
		// Delete RemoteTarget
		delErr := rsomp.Client.Delete(ctx, &rt)
		if delErr != nil {
			return delErr
		}

		// Remove association from RemoteUserBindings
		rubErr := rsomp.removeRemoteUserBindingAssociation(ctx, rt, *allRemoteUserBindings)
		if rubErr != nil {
			return rubErr
		}
	}

	return nil
}

func (rsomp *RemoteSyncerOneOrManyBranchPattern) removeRemoteUserBindingAssociation(ctx context.Context, remoteTarget syngit.RemoteTarget, remoteUserBindings syngit.RemoteUserBindingList) error {
	for _, rub := range remoteUserBindings.Items {
		newRemoteTargetRefs := []v1.ObjectReference{}
		spec := rub.Spec

		for _, associatedRemoteTargetRef := range rub.Spec.RemoteTargetRefs {
			if associatedRemoteTargetRef.Name != remoteTarget.Name || associatedRemoteTargetRef.Namespace != remoteTarget.Namespace {
				newRemoteTargetRefs = append(newRemoteTargetRefs, associatedRemoteTargetRef)
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

// Filter RemoteTargets with the same upstream & target repo & branches.
// The RemoteTarget is added to the output slice if and only if
// the RemoteTarget is not used by any other RemoteSyncer.
// The dependencies must be filtered out because they will not be
// recreated in this instance of the Pattern.
func (rsomp *RemoteSyncerOneOrManyBranchPattern) filterRemoteTargets(ctx context.Context, in syngit.RemoteTargetList) ([]syngit.RemoteTarget, error) {
	out := []syngit.RemoteTarget{}

	// Only filter for the branches that are actually not used anymore
	diff := slicesDifference(rsomp.GarbageTargetBranches, rsomp.TargetBranches)

	// Search for non-dependent branches
	deletable, depErr := rsomp.filterOutDependencies(ctx, diff)
	if depErr != nil {
		return nil, depErr
	}

	// Include all the branches
	branches := append(rsomp.TargetBranches, deletable...)

	for _, rt := range in.Items {
		spec := rt.Spec
		if spec.UpstreamRepository == rsomp.UpstreamRepo && spec.UpstreamBranch == rsomp.UpstreamBranch && spec.TargetRepository == rsomp.TargetRepository {
			for _, branch := range branches {
				if spec.TargetBranch == branch {
					out = append(out, rt)
				}
			}
		}
	}
	return out, nil
}

// Search for automatically created RemoteTargets that are NOT used by any other RemoteSyncer
func (rsomp *RemoteSyncerOneOrManyBranchPattern) filterOutDependencies(ctx context.Context, branches []string) ([]string, error) {
	out := []string{}

	remoteSyncers := &syngit.RemoteSyncerList{}
	selector := labels.NewSelector()
	requirement, reqErr := labels.NewRequirement(syngit.RtAnnotationBranches, selection.Exists, nil)
	if reqErr != nil {
		return nil, reqErr
	}
	selector.Add(*requirement)
	listOps := &client.ListOptions{
		Namespace:     rsomp.NamespacedName.Namespace,
		LabelSelector: labels.NewSelector(),
	}
	listErr := rsomp.Client.List(ctx, remoteSyncers, listOps)
	if listErr != nil {
		return nil, listErr
	}

	for _, remoteSyncer := range remoteSyncers.Items {
		remoteSyncerBranches := utils.GetBranchesFromAnnotation(remoteSyncer.Annotations[syngit.RtAnnotationBranches])
		for _, branch := range branches {
			if !slices.Contains(remoteSyncerBranches, branch) {
				out = append(out, branch)
			}
		}
	}

	return out, nil
}
