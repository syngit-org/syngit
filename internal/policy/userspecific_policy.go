package policy

import (
	"context"
	"math/rand"
	"time"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/utils"
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

func (p *UserSpecificPolicy) Applies(remoteSyncer *syngit.RemoteSyncer) bool {
	return remoteSyncer.Annotations[syngit.RtAnnotationKeyUserSpecific] != ""
}

func (p *UserSpecificPolicy) Reconcile(ctx context.Context, remoteSyncer *syngit.RemoteSyncer) (ctrl.Result, error) {
	rdm := time.Duration(rand.Intn(5)) * time.Second

	userSpecificAnnotation := remoteSyncer.Annotations[syngit.RtAnnotationKeyUserSpecific]

	managedRUBs, err := p.listManagedRUBs(ctx, remoteSyncer.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	existingRTs, err := p.listUserSpecificTargets(ctx, remoteSyncer.Namespace, remoteSyncer.Spec.RemoteRepository, remoteSyncer.Spec.DefaultBranch)
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

	activeUsers, result, err := p.reconcileUserTargets(ctx, remoteSyncer, managedRUBs, existingByUser, userSpecificAnnotation, rdm)
	if err != nil {
		return result, err
	}

	return p.pruneStaleTargets(ctx, remoteSyncer, existingByUser, activeUsers, rdm)
}

func (p *UserSpecificPolicy) Cleanup(ctx context.Context, remoteSyncer *syngit.RemoteSyncer) error {
	return p.cleanupUserSpecificTargets(ctx, remoteSyncer)
}

// reconcileUserTargets ensures a user-specific RemoteTarget exists for each
// managed RUB and is referenced from that RUB. Returns the set of users it
// touched so the caller can prune stale targets.
func (p *UserSpecificPolicy) reconcileUserTargets(
	ctx context.Context,
	remoteSyncer *syngit.RemoteSyncer,
	managedRUBs []syngit.RemoteUserBinding,
	existingByUser map[string]syngit.RemoteTarget,
	userSpecificAnnotation string,
	rdm time.Duration,
) (map[string]bool, ctrl.Result, error) {
	upstreamRepo := remoteSyncer.Spec.RemoteRepository
	upstreamBranch := remoteSyncer.Spec.DefaultBranch

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

		rt, err := p.buildUserSpecificTarget(remoteSyncer.Namespace, upstreamRepo, upstreamBranch, rawUsername, sanitizedUser, userSpecificAnnotation)
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
	remoteSyncer *syngit.RemoteSyncer,
	existingByUser map[string]syngit.RemoteTarget,
	activeUsers map[string]bool,
	rdm time.Duration,
) (ctrl.Result, error) {
	otherSyncers, err := p.getOtherSyncersWithUserSpecific(ctx, remoteSyncer.Namespace, remoteSyncer.Name)
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
		if err := utils.RemoveRemoteTargetRefFromManagedRUBs(ctx, p.Client, rt.Namespace, rt.Name); err != nil {
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

	rtName, err := utils.RemoteTargetNameConstructor(upstreamRepo, upstreamBranch, targetRepo, sanitizedUser)
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
			TargetBranch:       utils.SoftSanitize(rawUsername),
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
	return utils.MutateOrDeleteManagedRemoteUserBinding(ctx, p.Client,
		types.NamespacedName{Name: rub.Name, Namespace: rub.Namespace},
		func(fresh *syngit.RemoteUserBinding) error {
			utils.AddRemoteTargetRef(fresh, rtName)
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

// getOtherSyncersWithUserSpecific returns other RemoteSyncers with the user-specific annotation.
func (p *UserSpecificPolicy) getOtherSyncersWithUserSpecific(ctx context.Context, namespace, excludeName string) ([]syngit.RemoteSyncer, error) {
	rsList := &syngit.RemoteSyncerList{}
	if err := p.List(ctx, rsList, &client.ListOptions{Namespace: namespace}); err != nil {
		return nil, err
	}

	var others []syngit.RemoteSyncer
	for _, rs := range rsList.Items {
		if rs.Name != excludeName && rs.Annotations[syngit.RtAnnotationKeyUserSpecific] != "" {
			others = append(others, rs)
		}
	}
	return others, nil
}

// isRTUsedByOtherSyncer checks if another syncer with user-specific annotation has the same upstream.
func (p *UserSpecificPolicy) isRTUsedByOtherSyncer(rt syngit.RemoteTarget, otherSyncers []syngit.RemoteSyncer) bool {
	for _, rs := range otherSyncers {
		if rs.Spec.RemoteRepository == rt.Spec.UpstreamRepository && rs.Spec.DefaultBranch == rt.Spec.UpstreamBranch {
			return true
		}
	}
	return false
}

// cleanupUserSpecificTargets removes all user-specific RemoteTargets for this syncer (with cross-dependency check).
func (p *UserSpecificPolicy) cleanupUserSpecificTargets(ctx context.Context, remoteSyncer *syngit.RemoteSyncer) error {
	upstreamRepo := remoteSyncer.Spec.RemoteRepository
	upstreamBranch := remoteSyncer.Spec.DefaultBranch

	existingRTs, err := p.listUserSpecificTargets(ctx, remoteSyncer.Namespace, upstreamRepo, upstreamBranch)
	if err != nil {
		return err
	}

	otherSyncers, err := p.getOtherSyncersWithUserSpecific(ctx, remoteSyncer.Namespace, remoteSyncer.Name)
	if err != nil {
		return err
	}

	for _, rt := range existingRTs {
		if p.isRTUsedByOtherSyncer(rt, otherSyncers) {
			continue
		}
		if err := utils.RemoveRemoteTargetRefFromManagedRUBs(ctx, p.Client, rt.Namespace, rt.Name); err != nil {
			return err
		}
		if err := p.Delete(ctx, &rt); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
