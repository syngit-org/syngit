package controller

import (
	"context"
	"math/rand"
	"time"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const userSpecificPolicyFinalizer = "syngit.io/userspecific-policy"

// UserSpecificPolicyReconciler creates and manages per-user RemoteTargets
// for RemoteSyncers that have the user-specific annotation.
type UserSpecificPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=syngit.io,resources=remotesyncers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=syngit.io,resources=remotesyncers/finalizers,verbs=update
// +kubebuilder:rbac:groups=syngit.io,resources=remotetargets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syngit.io,resources=remoteuserbindings,verbs=get;list;watch;update;patch

func (r *UserSpecificPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	rdm := time.Duration(rand.New(rand.NewSource(3)).Intn(5)) * time.Second

	var remoteSyncer syngit.RemoteSyncer
	if err := r.Get(ctx, req.NamespacedName, &remoteSyncer); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconcile request",
		"resource", "userspecific-policy",
		"namespace", remoteSyncer.Namespace,
		"name", remoteSyncer.Name,
	)

	userSpecificAnnotation := remoteSyncer.Annotations[syngit.RtAnnotationKeyUserSpecific]

	// Handle deletion or annotation removal
	if !remoteSyncer.DeletionTimestamp.IsZero() || userSpecificAnnotation == "" {
		if err := r.cleanupUserSpecificTargets(ctx, &remoteSyncer); err != nil {
			return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
		}
		if controllerutil.RemoveFinalizer(&remoteSyncer, userSpecificPolicyFinalizer) {
			if err := r.Update(ctx, &remoteSyncer); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure finalizer is present
	if controllerutil.AddFinalizer(&remoteSyncer, userSpecificPolicyFinalizer) {
		if err := r.Update(ctx, &remoteSyncer); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: requeueAfter + rdm}, nil
	}

	upstreamRepo := remoteSyncer.Spec.RemoteRepository
	upstreamBranch := remoteSyncer.Spec.DefaultBranch

	// List all managed RemoteUserBindings in the namespace
	managedRUBs, err := r.listManagedRUBs(ctx, remoteSyncer.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// List existing user-specific RemoteTargets for this syncer's upstream
	existingRTs, err := r.listUserSpecificTargets(ctx, remoteSyncer.Namespace, upstreamRepo, upstreamBranch)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Build index of existing RTs by sanitized username
	existingByUser := map[string]syngit.RemoteTarget{}
	for _, rt := range existingRTs {
		sanitizedUser := rt.Labels[syngit.K8sUserLabelKey]
		if sanitizedUser != "" {
			existingByUser[sanitizedUser] = rt
		}
	}

	// For each managed RUB, ensure a user-specific RemoteTarget exists
	activeUsers := map[string]bool{}
	for i := range managedRUBs {
		rub := &managedRUBs[i]
		sanitizedUser := rub.Labels[syngit.K8sUserLabelKey]
		if sanitizedUser == "" {
			continue
		}
		activeUsers[sanitizedUser] = true
		rawUsername := rub.Spec.Subject.Name

		if _, exists := existingByUser[sanitizedUser]; exists {
			// Already exists, ensure it's referenced in the RUB
			rt := existingByUser[sanitizedUser]
			if err := r.ensureRTRefInRUB(ctx, rub, rt.Name); err != nil {
				return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
			}
			continue
		}

		// Create user-specific RemoteTarget
		rt, err := r.buildUserSpecificTarget(remoteSyncer.Namespace, upstreamRepo, upstreamBranch, rawUsername, sanitizedUser, userSpecificAnnotation)
		if err != nil {
			return ctrl.Result{}, err
		}

		if createErr := r.Create(ctx, rt); createErr != nil {
			if !apierrors.IsAlreadyExists(createErr) {
				return ctrl.Result{}, createErr
			}
		}
		// Ensure the reference is in the RUB
		if err := r.ensureRTRefInRUB(ctx, rub, rt.Name); err != nil {
			return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
		}
	}

	// Clean up user-specific RTs for users that no longer have a managed RUB
	otherSyncers, err := r.getOtherSyncersWithUserSpecific(ctx, remoteSyncer.Namespace, remoteSyncer.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	for userLabel, rt := range existingByUser {
		if activeUsers[userLabel] {
			continue
		}
		// Check if another syncer with same upstream still uses this RT
		if r.isRTUsedByOtherSyncer(rt, otherSyncers) {
			continue
		}
		// Remove RT reference from managed RUBs, then delete the RT
		if err := utils.RemoveRemoteTargetRefFromManagedRUBs(ctx, r.Client, rt.Namespace, rt.Name); err != nil {
			return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
		}
		if err := r.Delete(ctx, &rt); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// buildUserSpecificTarget creates a RemoteTarget for a specific user.
func (r *UserSpecificPolicyReconciler) buildUserSpecificTarget(namespace, upstreamRepo, upstreamBranch, rawUsername, sanitizedUser, annotationValue string) (*syngit.RemoteTarget, error) {
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
func (r *UserSpecificPolicyReconciler) ensureRTRefInRUB(ctx context.Context, rub *syngit.RemoteUserBinding, rtName string) error {
	for _, ref := range rub.Spec.RemoteTargetRefs {
		if ref.Name == rtName {
			return nil
		}
	}
	spec := *rub.Spec.DeepCopy()
	spec.RemoteTargetRefs = append(spec.RemoteTargetRefs, corev1.ObjectReference{Name: rtName})
	if err := utils.UpdateOrDeleteManagedRemoteUserBinding(ctx, r.Client, spec, *rub); err != nil {
		return err
	}
	return nil
}

// listManagedRUBs returns all managed RemoteUserBindings in the namespace.
func (r *UserSpecificPolicyReconciler) listManagedRUBs(ctx context.Context, namespace string) ([]syngit.RemoteUserBinding, error) {
	rubList := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		Namespace: namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
		}),
	}
	if err := r.List(ctx, rubList, listOps); err != nil {
		return nil, err
	}
	return rubList.Items, nil
}

// listUserSpecificTargets lists user-specific managed RemoteTargets matching the given upstream.
func (r *UserSpecificPolicyReconciler) listUserSpecificTargets(ctx context.Context, namespace, upstreamRepo, upstreamBranch string) ([]syngit.RemoteTarget, error) {
	rtList := &syngit.RemoteTargetList{}
	listOps := &client.ListOptions{
		Namespace: namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.RtLabelKeyPolicy:  syngit.RtLabelValueOneUserOneBranch,
		}),
	}
	if err := r.List(ctx, rtList, listOps); err != nil {
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
func (r *UserSpecificPolicyReconciler) getOtherSyncersWithUserSpecific(ctx context.Context, namespace, excludeName string) ([]syngit.RemoteSyncer, error) {
	rsList := &syngit.RemoteSyncerList{}
	if err := r.List(ctx, rsList, &client.ListOptions{Namespace: namespace}); err != nil {
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
func (r *UserSpecificPolicyReconciler) isRTUsedByOtherSyncer(rt syngit.RemoteTarget, otherSyncers []syngit.RemoteSyncer) bool {
	for _, rs := range otherSyncers {
		if rs.Spec.RemoteRepository == rt.Spec.UpstreamRepository && rs.Spec.DefaultBranch == rt.Spec.UpstreamBranch {
			return true
		}
	}
	return false
}

// cleanupUserSpecificTargets removes all user-specific RemoteTargets for this syncer (with cross-dependency check).
func (r *UserSpecificPolicyReconciler) cleanupUserSpecificTargets(ctx context.Context, remoteSyncer *syngit.RemoteSyncer) error {
	upstreamRepo := remoteSyncer.Spec.RemoteRepository
	upstreamBranch := remoteSyncer.Spec.DefaultBranch

	existingRTs, err := r.listUserSpecificTargets(ctx, remoteSyncer.Namespace, upstreamRepo, upstreamBranch)
	if err != nil {
		return err
	}

	otherSyncers, err := r.getOtherSyncersWithUserSpecific(ctx, remoteSyncer.Namespace, remoteSyncer.Name)
	if err != nil {
		return err
	}

	for _, rt := range existingRTs {
		if r.isRTUsedByOtherSyncer(rt, otherSyncers) {
			continue
		}
		if err := utils.RemoveRemoteTargetRefFromManagedRUBs(ctx, r.Client, rt.Namespace, rt.Name); err != nil {
			return err
		}
		if err := r.Delete(ctx, &rt); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// findRemoteSyncersForRUB maps RemoteUserBinding changes to RemoteSyncer reconcile requests.
func (r *UserSpecificPolicyReconciler) findRemoteSyncersForRUB(ctx context.Context, obj client.Object) []reconcile.Request {
	rub, ok := obj.(*syngit.RemoteUserBinding)
	if !ok {
		return nil
	}

	// Only care about managed RUBs
	if rub.Labels[syngit.ManagedByLabelKey] != syngit.ManagedByLabelValue {
		return nil
	}

	// Find all RemoteSyncers in the namespace with user-specific annotation
	rsList := &syngit.RemoteSyncerList{}
	if err := r.List(ctx, rsList, &client.ListOptions{Namespace: rub.Namespace}); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, rs := range rsList.Items {
		if rs.Annotations[syngit.RtAnnotationKeyUserSpecific] != "" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				},
			})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *UserSpecificPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&syngit.RemoteSyncer{}).
		Watches(
			&syngit.RemoteUserBinding{},
			handler.EnqueueRequestsFromMapFunc(r.findRemoteSyncersForRUB),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Named("userspecific-policy").
		Complete(r)
}
