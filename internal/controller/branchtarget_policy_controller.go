package controller

import (
	"context"
	"math/rand"
	"slices"
	"time"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const branchTargetPolicyFinalizer = "syngit.io/branchtarget-policy"

// BranchTargetPolicyReconciler creates and manages RemoteTargets
// for RemoteSyncers that have the one-or-many-branches annotation.
type BranchTargetPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=syngit.io,resources=remotesyncers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=syngit.io,resources=remotesyncers/finalizers,verbs=update
// +kubebuilder:rbac:groups=syngit.io,resources=remotetargets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syngit.io,resources=remoteuserbindings,verbs=get;list;watch;update;patch

func (r *BranchTargetPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	rdm := time.Duration(rand.New(rand.NewSource(2)).Intn(5)) * time.Second

	var remoteSyncer syngit.RemoteSyncer
	if err := r.Get(ctx, req.NamespacedName, &remoteSyncer); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconcile request",
		"resource", "branchtarget-policy",
		"namespace", remoteSyncer.Namespace,
		"name", remoteSyncer.Name,
	)

	annotation := remoteSyncer.Annotations[syngit.RtAnnotationKeyOneOrManyBranches]
	desiredBranches := utils.GetBranchesFromAnnotation(annotation)

	// Handle deletion or annotation removal
	if !remoteSyncer.DeletionTimestamp.IsZero() || len(desiredBranches) == 0 {
		if err := r.cleanupBranchTargets(ctx, &remoteSyncer); err != nil {
			return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
		}
		if controllerutil.RemoveFinalizer(&remoteSyncer, branchTargetPolicyFinalizer) {
			if err := r.Update(ctx, &remoteSyncer); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure finalizer is present
	if controllerutil.AddFinalizer(&remoteSyncer, branchTargetPolicyFinalizer) {
		if err := r.Update(ctx, &remoteSyncer); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: requeueAfter + rdm}, nil
	}

	upstreamRepo := remoteSyncer.Spec.RemoteRepository
	upstreamBranch := remoteSyncer.Spec.DefaultBranch

	// List existing managed RemoteTargets with one-or-many-branches label
	existingRTs, err := r.listManagedBranchTargets(ctx, remoteSyncer.Namespace)
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
		rt, nameErr := r.buildRemoteTarget(remoteSyncer.Namespace, upstreamRepo, upstreamBranch, branch)
		if nameErr != nil {
			return ctrl.Result{}, nameErr
		}
		if createErr := r.Create(ctx, rt); createErr != nil {
			if !apierrors.IsAlreadyExists(createErr) {
				return ctrl.Result{}, createErr
			}
		}
	}

	// Delete orphaned RemoteTargets (only if no other RemoteSyncer depends on them)
	otherSyncers, err := r.getOtherSyncersWithBranchPolicy(ctx, remoteSyncer.Namespace, remoteSyncer.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	for branch, rt := range existingBranches {
		if !slices.Contains(desiredBranches, branch) {
			if !r.isBranchUsedByOtherSyncer(branch, upstreamRepo, upstreamBranch, otherSyncers) {
				if err := r.Delete(ctx, &rt); err != nil && !apierrors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
				if err := utils.RemoveRemoteTargetRefFromManagedRUBs(ctx, r.Client, rt.Namespace, rt.Name); err != nil {
					return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
				}
			}
		}
	}

	return ctrl.Result{}, nil
}

// buildRemoteTarget constructs a RemoteTarget for a branch.
func (r *BranchTargetPolicyReconciler) buildRemoteTarget(namespace, upstreamRepo, upstreamBranch, targetBranch string) (*syngit.RemoteTarget, error) {
	name, err := utils.RemoteTargetNameConstructor(upstreamRepo, upstreamBranch, upstreamRepo, targetBranch)
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
func (r *BranchTargetPolicyReconciler) listManagedBranchTargets(ctx context.Context, namespace string) ([]syngit.RemoteTarget, error) {
	rtList := &syngit.RemoteTargetList{}
	listOps := &client.ListOptions{
		Namespace: namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.RtLabelKeyPolicy:  syngit.RtLabelValueOneOrManyBranches,
		}),
	}
	if err := r.List(ctx, rtList, listOps); err != nil {
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

// getOtherSyncersWithBranchPolicy returns other RemoteSyncers in the namespace that have the OMB annotation.
func (r *BranchTargetPolicyReconciler) getOtherSyncersWithBranchPolicy(ctx context.Context, namespace, excludeName string) ([]syngit.RemoteSyncer, error) {
	rsList := &syngit.RemoteSyncerList{}
	if err := r.List(ctx, rsList, &client.ListOptions{Namespace: namespace}); err != nil {
		return nil, err
	}

	var others []syngit.RemoteSyncer
	for _, rs := range rsList.Items {
		if rs.Name != excludeName && rs.Annotations[syngit.RtAnnotationKeyOneOrManyBranches] != "" {
			others = append(others, rs)
		}
	}
	return others, nil
}

// isBranchUsedByOtherSyncer checks if another RemoteSyncer references the same upstream+branch combination.
func (r *BranchTargetPolicyReconciler) isBranchUsedByOtherSyncer(branch, upstreamRepo, upstreamBranch string, otherSyncers []syngit.RemoteSyncer) bool {
	for _, rs := range otherSyncers {
		if rs.Spec.RemoteRepository == upstreamRepo && rs.Spec.DefaultBranch == upstreamBranch {
			branches := utils.GetBranchesFromAnnotation(rs.Annotations[syngit.RtAnnotationKeyOneOrManyBranches])
			if slices.Contains(branches, branch) {
				return true
			}
		}
	}
	return false
}

// cleanupBranchTargets removes all managed branch RemoteTargets for this syncer (with cross-dependency check).
func (r *BranchTargetPolicyReconciler) cleanupBranchTargets(ctx context.Context, remoteSyncer *syngit.RemoteSyncer) error {
	upstreamRepo := remoteSyncer.Spec.RemoteRepository
	upstreamBranch := remoteSyncer.Spec.DefaultBranch

	existingRTs, err := r.listManagedBranchTargets(ctx, remoteSyncer.Namespace)
	if err != nil {
		return err
	}

	matchingRTs := filterByUpstream(existingRTs, upstreamRepo, upstreamBranch)

	otherSyncers, err := r.getOtherSyncersWithBranchPolicy(ctx, remoteSyncer.Namespace, remoteSyncer.Name)
	if err != nil {
		return err
	}

	for _, rt := range matchingRTs {
		if !r.isBranchUsedByOtherSyncer(rt.Spec.TargetBranch, upstreamRepo, upstreamBranch, otherSyncers) {
			if err := r.Delete(ctx, &rt); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			if err := utils.RemoveRemoteTargetRefFromManagedRUBs(ctx, r.Client, rt.Namespace, rt.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BranchTargetPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&syngit.RemoteSyncer{}).
		Named("branchtarget-policy").
		Complete(r)
}
