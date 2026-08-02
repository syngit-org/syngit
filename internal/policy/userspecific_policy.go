package policy

import (
	"context"
	"math/rand"
	"time"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"github.com/syngit-org/syngit/pkg/managed"
	"github.com/syngit-org/syngit/pkg/naming"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const userSpecificPolicyFinalizer = "syngit.io/userspecific-policy"

// UserSpecificPolicy creates and manages the per-user RemoteTargets of a
// RemoteSyncer that carries the user-specific annotation. It implements
// policy.Policy[*syngit.RemoteSyncer] and is run by RemoteSyncerReconciler, the
// single controller that owns RemoteSyncer.
type UserSpecificPolicy struct {
	client.Client
}

func (p *UserSpecificPolicy) Name() string { return "userspecific-policy" }

func (p *UserSpecificPolicy) Finalizer() string { return userSpecificPolicyFinalizer }

func (p *UserSpecificPolicy) Applies(syncer syngit.Syncer) bool {
	return syncer.GetAnnotations()[syngit.RtAnnotationKeyUserSpecific] != ""
}

func (p *UserSpecificPolicy) Reconcile(ctx context.Context, syncer syngit.Syncer) (ctrl.Result, error) {
	rdm := time.Duration(rand.Intn(5)) * time.Second

	userSpecificAnnotation := syncer.GetAnnotations()[syngit.RtAnnotationKeyUserSpecific]

	managedRUBs, err := p.listManagedRUBs(ctx, syncer.IdentityNamespace())
	if err != nil {
		return ctrl.Result{}, err
	}

	existingRTs, err := p.listUserSpecificTargets(ctx, syncer.IdentityNamespace(), syncer.SyncerSpec().RemoteRepository, syncer.SyncerSpec().DefaultBranch)
	if err != nil {
		return ctrl.Result{}, err
	}

	existingByUser := map[string]syngit.RemoteTarget{}
	for _, rt := range existingRTs {
		sanitizedUser := rt.Labels[syngit.K8sUserLabelKey]
		if sanitizedUser != "" {
			existingByUser[sanitizedUser] = rt
		}
	}

	activeUsers, result, err := p.reconcileUserTargets(ctx, syncer, managedRUBs, existingByUser, userSpecificAnnotation, rdm)
	if err != nil {
		return result, err
	}

	return p.pruneStaleTargets(ctx, syncer, existingByUser, activeUsers, rdm)
}

func (p *UserSpecificPolicy) Cleanup(ctx context.Context, syncer syngit.Syncer) error {
	return p.cleanupUserSpecificTargets(ctx, syncer)
}

// reconcileUserTargets ensures a user-specific RemoteTarget exists for each
// managed RUB and is referenced from that RUB. Returns the set of users it
// touched so the caller can prune stale targets.
func (p *UserSpecificPolicy) reconcileUserTargets(
	ctx context.Context,
	syncer syngit.Syncer,
	managedRUBs []syngit.RemoteUserBinding,
	existingByUser map[string]syngit.RemoteTarget,
	userSpecificAnnotation string,
	rdm time.Duration,
) (map[string]bool, ctrl.Result, error) {
	upstreamRepo := syncer.SyncerSpec().RemoteRepository
	upstreamBranch := syncer.SyncerSpec().DefaultBranch

	activeUsers := map[string]bool{}
	for i := range managedRUBs {
		rub := &managedRUBs[i]
		sanitizedUser := rub.Labels[syngit.K8sUserLabelKey]
		if sanitizedUser == "" {
			continue
		}
		activeUsers[sanitizedUser] = true
		rawUsername := rub.Spec.Subject.Name

		if rt, exists := existingByUser[sanitizedUser]; exists {
			if err := p.ensureRTRefInRUB(ctx, rub, rt.Name); err != nil {
				return activeUsers, ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
			}
			continue
		}

		rt, err := p.buildUserSpecificTarget(syncer.IdentityNamespace(), upstreamRepo, upstreamBranch, rawUsername, sanitizedUser, userSpecificAnnotation)
		if err != nil {
			return activeUsers, ctrl.Result{}, err
		}

		if createErr := p.Create(ctx, rt); createErr != nil {
			if !apierrors.IsAlreadyExists(createErr) {
				return activeUsers, ctrl.Result{}, createErr
			}
		}
		if err := p.ensureRTRefInRUB(ctx, rub, rt.Name); err != nil {
			return activeUsers, ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
		}
	}
	return activeUsers, ctrl.Result{}, nil
}

// pruneStaleTargets deletes user-specific RemoteTargets for users that no
// longer have a managed RUB, unless another user-specific syncer with the same
// upstream still uses them.
func (p *UserSpecificPolicy) pruneStaleTargets(
	ctx context.Context,
	syncer syngit.Syncer,
	existingByUser map[string]syngit.RemoteTarget,
	activeUsers map[string]bool,
	rdm time.Duration,
) (ctrl.Result, error) {
	otherSyncers, err := p.getOtherSyncersWithUserSpecific(ctx, syncer)
	if err != nil {
		return ctrl.Result{}, err
	}

	for userLabel, rt := range existingByUser {
		if activeUsers[userLabel] {
			continue
		}
		if p.isRTUsedByOtherSyncer(rt, otherSyncers) {
			continue
		}
		if err := managed.RemoveRemoteTargetRefFromRUBs(ctx, p.Client, rt.Namespace, rt.Name); err != nil {
			return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
		}
		if err := p.Delete(ctx, &rt); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// buildUserSpecificTarget creates a RemoteTarget for a specific user.
func (p *UserSpecificPolicy) buildUserSpecificTarget(namespace, upstreamRepo, upstreamBranch, rawUsername, sanitizedUser, annotationValue string) (*syngit.RemoteTarget, error) {
	targetRepo := upstreamRepo
	if annotationValue == string(syngit.RtAnnotationValueOneUserOneFork) {
		targetRepo = ""
	}

	rtName, err := naming.RemoteTargetName(upstreamRepo, upstreamBranch, targetRepo, sanitizedUser)
	if err != nil {
		return nil, err
	}

	rt := &syngit.RemoteTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rtName,
			Namespace: namespace,
			Labels: map[string]string{
				syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
				syngit.RtLabelKeyPolicy:  syngit.RtLabelValueOneUserOneBranch,
				syngit.K8sUserLabelKey:   sanitizedUser,
			},
		},
		Spec: syngit.RemoteTargetSpec{
			UpstreamRepository: upstreamRepo,
			UpstreamBranch:     upstreamBranch,
			TargetRepository:   targetRepo,
			TargetBranch:       naming.SoftSanitize(rawUsername),
			MergeStrategy:      syngit.TryFastForwardOrHardReset,
		},
	}

	if targetRepo == "" {
		rt.Annotations = map[string]string{
			syngit.RtLabelKeyAllowInjection: "true",
		}
	}

	return rt, nil
}

// ensureRTRefInRUB ensures the RemoteTarget is referenced in the RUB and persists the change.
func (p *UserSpecificPolicy) ensureRTRefInRUB(ctx context.Context, rub *syngit.RemoteUserBinding, rtName string) error {
	return managed.MutateOrDeleteRemoteUserBinding(ctx, p.Client,
		types.NamespacedName{Name: rub.Name, Namespace: rub.Namespace},
		func(fresh *syngit.RemoteUserBinding) error {
			managed.AddRemoteTargetRef(fresh, rtName)
			return nil
		})
}

// listManagedRUBs returns all managed RemoteUserBindings in the namespace.
func (p *UserSpecificPolicy) listManagedRUBs(ctx context.Context, namespace string) ([]syngit.RemoteUserBinding, error) {
	rubList := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		Namespace: namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
		}),
	}
	if err := p.List(ctx, rubList, listOps); err != nil {
		return nil, err
	}
	return rubList.Items, nil
}

// listUserSpecificTargets lists user-specific managed RemoteTargets matching the given upstream.
func (p *UserSpecificPolicy) listUserSpecificTargets(ctx context.Context, namespace, upstreamRepo, upstreamBranch string) ([]syngit.RemoteTarget, error) {
	rtList := &syngit.RemoteTargetList{}
	listOps := &client.ListOptions{
		Namespace: namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.RtLabelKeyPolicy:  syngit.RtLabelValueOneUserOneBranch,
		}),
	}
	if err := p.List(ctx, rtList, listOps); err != nil {
		return nil, err
	}

	var filtered []syngit.RemoteTarget
	for _, rt := range rtList.Items {
		if rt.Spec.UpstreamRepository == upstreamRepo && rt.Spec.UpstreamBranch == upstreamBranch {
			filtered = append(filtered, rt)
		}
	}
	return filtered, nil
}

// getOtherSyncersWithUserSpecific returns the other syncers, of either kind,
// sharing this one's identity namespace and carrying the user-specific annotation.
func (p *UserSpecificPolicy) getOtherSyncersWithUserSpecific(ctx context.Context, syncer syngit.Syncer) ([]syngit.Syncer, error) {
	return getOtherSyncersWith(ctx, p.Client, syncer, syngit.RtAnnotationKeyUserSpecific)
}

// isRTUsedByOtherSyncer checks if another syncer with user-specific annotation has the same upstream.
func (p *UserSpecificPolicy) isRTUsedByOtherSyncer(rt syngit.RemoteTarget, otherSyncers []syngit.Syncer) bool {
	for _, rs := range otherSyncers {
		spec := rs.SyncerSpec()
		if spec.RemoteRepository == rt.Spec.UpstreamRepository && spec.DefaultBranch == rt.Spec.UpstreamBranch {
			return true
		}
	}
	return false
}

// cleanupUserSpecificTargets removes all user-specific RemoteTargets for this syncer (with cross-dependency check).
func (p *UserSpecificPolicy) cleanupUserSpecificTargets(ctx context.Context, syncer syngit.Syncer) error {
	upstreamRepo := syncer.SyncerSpec().RemoteRepository
	upstreamBranch := syncer.SyncerSpec().DefaultBranch

	existingRTs, err := p.listUserSpecificTargets(ctx, syncer.IdentityNamespace(), upstreamRepo, upstreamBranch)
	if err != nil {
		return err
	}

	otherSyncers, err := p.getOtherSyncersWithUserSpecific(ctx, syncer)
	if err != nil {
		return err
	}

	for _, rt := range existingRTs {
		if p.isRTUsedByOtherSyncer(rt, otherSyncers) {
			continue
		}
		if err := managed.RemoveRemoteTargetRefFromRUBs(ctx, p.Client, rt.Namespace, rt.Name); err != nil {
			return err
		}
		if err := p.Delete(ctx, &rt); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
