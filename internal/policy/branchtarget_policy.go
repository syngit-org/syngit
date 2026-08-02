package policy

import (
	"context"
	"math/rand"
	"slices"
	"time"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"github.com/syngit-org/syngit/pkg/managed"
	"github.com/syngit-org/syngit/pkg/naming"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const branchTargetPolicyFinalizer = "syngit.io/branchtarget-policy"

// +kubebuilder:rbac:groups=syngit.io,resources=remotetargets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syngit.io,resources=remoteuserbindings,verbs=get;list;watch;update;patch

// BranchTargetPolicy creates and manages the RemoteTargets of a RemoteSyncer that
// carries the one-or-many-branches annotation. It implements
// policy.Policy[*syngit.RemoteSyncer] and is run by RemoteSyncerReconciler, the
// single controller that owns RemoteSyncer.
type BranchTargetPolicy struct {
	client.Client
}

func (p *BranchTargetPolicy) Name() string { return "branchtarget-policy" }

func (p *BranchTargetPolicy) Finalizer() string { return branchTargetPolicyFinalizer }

func (p *BranchTargetPolicy) Applies(syncer syngit.Syncer) bool {
	return len(managed.BranchesFromAnnotation(syncer.GetAnnotations()[syngit.RtAnnotationKeyOneOrManyBranches])) > 0
}

func (p *BranchTargetPolicy) Reconcile(ctx context.Context, syncer syngit.Syncer) (ctrl.Result, error) {
	rdm := time.Duration(rand.Intn(5)) * time.Second

	desiredBranches := managed.BranchesFromAnnotation(syncer.GetAnnotations()[syngit.RtAnnotationKeyOneOrManyBranches])

	upstreamRepo := syncer.SyncerSpec().RemoteRepository
	upstreamBranch := syncer.SyncerSpec().DefaultBranch

	// List existing managed RemoteTargets with one-or-many-branches label
	existingRTs, err := p.listManagedBranchTargets(ctx, syncer.IdentityNamespace())
	if err != nil {
		return ctrl.Result{}, err
	}

	// Filter to those matching this syncer's upstream
	matchingRTs := filterByUpstream(existingRTs, upstreamRepo, upstreamBranch)

	// Determine what exists
	existingBranches := map[string]syngit.RemoteTarget{}
	for _, rt := range matchingRTs {
		existingBranches[rt.Spec.TargetBranch] = rt
	}

	// Create missing RemoteTargets
	for _, branch := range desiredBranches {
		if _, exists := existingBranches[branch]; exists {
			continue
		}
		rt, nameErr := p.buildRemoteTarget(syncer.IdentityNamespace(), upstreamRepo, upstreamBranch, branch)
		if nameErr != nil {
			return ctrl.Result{}, nameErr
		}
		if createErr := p.Create(ctx, rt); createErr != nil {
			if !apierrors.IsAlreadyExists(createErr) {
				return ctrl.Result{}, createErr
			}
		}
	}

	// Delete orphaned RemoteTargets (only if no other RemoteSyncer depends on them)
	otherSyncers, err := p.getOtherSyncersWithBranchPolicy(ctx, syncer)
	if err != nil {
		return ctrl.Result{}, err
	}

	for branch, rt := range existingBranches {
		if !slices.Contains(desiredBranches, branch) {
			if !p.isBranchUsedByOtherSyncer(branch, upstreamRepo, upstreamBranch, otherSyncers) {
				if err := p.Delete(ctx, &rt); err != nil && !apierrors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
				if err := managed.RemoveRemoteTargetRefFromRUBs(ctx, p.Client, rt.Namespace, rt.Name); err != nil {
					return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
				}
			}
		}
	}

	return ctrl.Result{}, nil
}

func (p *BranchTargetPolicy) Cleanup(ctx context.Context, syncer syngit.Syncer) error {
	return p.cleanupBranchTargets(ctx, syncer)
}

// buildRemoteTarget constructs a RemoteTarget for a branch.
func (p *BranchTargetPolicy) buildRemoteTarget(namespace, upstreamRepo, upstreamBranch, targetBranch string) (*syngit.RemoteTarget, error) {
	name, err := naming.RemoteTargetName(upstreamRepo, upstreamBranch, upstreamRepo, targetBranch)
	if err != nil {
		return nil, err
	}

	mergeStrategy := syngit.TryFastForwardOrHardReset
	if upstreamBranch == targetBranch {
		mergeStrategy = ""
	}

	return &syngit.RemoteTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
				syngit.RtLabelKeyBranch:  targetBranch,
				syngit.RtLabelKeyPolicy:  syngit.RtLabelValueOneOrManyBranches,
			},
		},
		Spec: syngit.RemoteTargetSpec{
			UpstreamRepository: upstreamRepo,
			UpstreamBranch:     upstreamBranch,
			TargetRepository:   upstreamRepo,
			TargetBranch:       targetBranch,
			MergeStrategy:      mergeStrategy,
		},
	}, nil
}

// listManagedBranchTargets lists all managed one-or-many-branches RemoteTargets in a namespace.
func (p *BranchTargetPolicy) listManagedBranchTargets(ctx context.Context, namespace string) ([]syngit.RemoteTarget, error) {
	rtList := &syngit.RemoteTargetList{}
	listOps := &client.ListOptions{
		Namespace: namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.RtLabelKeyPolicy:  syngit.RtLabelValueOneOrManyBranches,
		}),
	}
	if err := p.List(ctx, rtList, listOps); err != nil {
		return nil, err
	}
	return rtList.Items, nil
}

// filterByUpstream filters RemoteTargets that match the given upstream repo and branch.
func filterByUpstream(rts []syngit.RemoteTarget, upstreamRepo, upstreamBranch string) []syngit.RemoteTarget {
	var filtered []syngit.RemoteTarget
	for _, rt := range rts {
		if rt.Spec.UpstreamRepository == upstreamRepo && rt.Spec.UpstreamBranch == upstreamBranch {
			filtered = append(filtered, rt)
		}
	}
	return filtered
}

// getOtherSyncersWithBranchPolicy returns the other syncers, of either kind,
// sharing this one's identity namespace and carrying the OMB annotation.
func (p *BranchTargetPolicy) getOtherSyncersWithBranchPolicy(ctx context.Context, syncer syngit.Syncer) ([]syngit.Syncer, error) {
	return getOtherSyncersWith(ctx, p.Client, syncer, syngit.RtAnnotationKeyOneOrManyBranches)
}

// isBranchUsedByOtherSyncer checks if another syncer references the same upstream+branch combination.
func (p *BranchTargetPolicy) isBranchUsedByOtherSyncer(branch, upstreamRepo, upstreamBranch string, otherSyncers []syngit.Syncer) bool {
	for _, rs := range otherSyncers {
		spec := rs.SyncerSpec()
		if spec.RemoteRepository == upstreamRepo && spec.DefaultBranch == upstreamBranch {
			branches := managed.BranchesFromAnnotation(rs.GetAnnotations()[syngit.RtAnnotationKeyOneOrManyBranches])
			if slices.Contains(branches, branch) {
				return true
			}
		}
	}
	return false
}

// cleanupBranchTargets removes all managed branch RemoteTargets for this syncer (with cross-dependency check).
func (p *BranchTargetPolicy) cleanupBranchTargets(ctx context.Context, syncer syngit.Syncer) error {
	upstreamRepo := syncer.SyncerSpec().RemoteRepository
	upstreamBranch := syncer.SyncerSpec().DefaultBranch

	existingRTs, err := p.listManagedBranchTargets(ctx, syncer.IdentityNamespace())
	if err != nil {
		return err
	}

	matchingRTs := filterByUpstream(existingRTs, upstreamRepo, upstreamBranch)

	otherSyncers, err := p.getOtherSyncersWithBranchPolicy(ctx, syncer)
	if err != nil {
		return err
	}

	for _, rt := range matchingRTs {
		if !p.isBranchUsedByOtherSyncer(rt.Spec.TargetBranch, upstreamRepo, upstreamBranch, otherSyncers) {
			if err := p.Delete(ctx, &rt); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			if err := managed.RemoveRemoteTargetRefFromRUBs(ctx, p.Client, rt.Namespace, rt.Name); err != nil {
				return err
			}
		}
	}

	return nil
}
