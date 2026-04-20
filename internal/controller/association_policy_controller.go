package controller

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

const associationPolicyFinalizer = "syngit.io/association-policy"

// AssociationPolicyReconciler manages RemoteUserBindings for RemoteUsers
// that have the managed annotation set.
type AssociationPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=syngit.io,resources=remoteusers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=syngit.io,resources=remoteusers/finalizers,verbs=update
// +kubebuilder:rbac:groups=syngit.io,resources=remoteuserbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syngit.io,resources=remotetargets,verbs=get;list;watch

func (r *AssociationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	rdm := time.Duration(rand.New(rand.NewSource(1)).Intn(5)) * time.Second

	var remoteUser syngit.RemoteUser
	if err := r.Get(ctx, req.NamespacedName, &remoteUser); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconcile request",
		"resource", "association-policy",
		"namespace", remoteUser.Namespace,
		"name", remoteUser.Name,
	)

	isEnabled := remoteUser.Annotations[syngit.RubAnnotationKeyManaged] == "true" // nolint:goconst
	sanitizedUsername := remoteUser.Labels[syngit.K8sUserLabelKey]
	rawUsername := remoteUser.Annotations[syngit.K8sUserLabelKey]

	// Handle deletion with finalizer
	if !remoteUser.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&remoteUser, associationPolicyFinalizer) {
			if err := r.cleanupAssociation(ctx, &remoteUser, sanitizedUsername); err != nil {
				return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
			}
			controllerutil.RemoveFinalizer(&remoteUser, associationPolicyFinalizer)
			if err := r.Update(ctx, &remoteUser); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// If the managed annotation is not set, remove finalizer and clean up if needed
	if !isEnabled {
		if err := r.cleanupAssociation(ctx, &remoteUser, sanitizedUsername); err != nil {
			return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
		}
		if controllerutil.RemoveFinalizer(&remoteUser, associationPolicyFinalizer) {
			if err := r.Update(ctx, &remoteUser); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if sanitizedUsername == "" || rawUsername == "" {
		logger.Info("RemoteUser has managed annotation but no k8s-user label/annotation, waiting for mutating webhook to stamp it")
		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(&remoteUser, associationPolicyFinalizer) {
		if err := r.Update(ctx, &remoteUser); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Find or create the managed RemoteUserBinding for this user
	rub, err := r.findOrCreateManagedRUB(ctx, &remoteUser, sanitizedUsername, rawUsername)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Ensure the RemoteUser is in the RUB's remoteUserRefs
	if err := r.ensureRemoteUserRef(ctx, rub, remoteUser.Name); err != nil {
		return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
	}

	// Search for RemoteTargets with one-or-many-branches label and ensure they're in the RUB
	if err := r.associateExistingRemoteTargets(ctx, rub); err != nil {
		return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
	}

	return ctrl.Result{}, nil
}

// findOrCreateManagedRUB finds the managed RemoteUserBinding for a user, or creates one.
func (r *AssociationPolicyReconciler) findOrCreateManagedRUB(ctx context.Context, remoteUser *syngit.RemoteUser, sanitizedUsername, rawUsername string) (*syngit.RemoteUserBinding, error) {
	rubList := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.K8sUserLabelKey:   sanitizedUsername,
		}),
		Namespace: remoteUser.Namespace,
	}
	if err := r.List(ctx, rubList, listOps); err != nil {
		return nil, err
	}

	if len(rubList.Items) > 0 {
		freshRub := &syngit.RemoteUserBinding{}
		if err := r.Get(ctx, types.NamespacedName{Name: rubList.Items[0].Name, Namespace: remoteUser.Namespace}, freshRub); err != nil {
			return nil, err
		}
		return freshRub, nil
	}

	// Create a new managed RUB
	baseName := syngit.RubNamePrefix + "-" + sanitizedUsername
	name, err := r.generateUniqueName(ctx, baseName, remoteUser.Namespace)
	if err != nil {
		return nil, err
	}

	rub := &syngit.RemoteUserBinding{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: remoteUser.Namespace,
			Labels: map[string]string{
				syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
				syngit.K8sUserLabelKey:   sanitizedUsername,
			},
		},
		Spec: syngit.RemoteUserBindingSpec{
			Subject: rbacv1.Subject{
				Kind: "User",
				Name: rawUsername,
			},
			RemoteUserRefs: []corev1.ObjectReference{
				{
					Name: remoteUser.Name,
				},
			},
		},
	}

	if err := r.Create(ctx, rub); err != nil {
		if apierrors.IsAlreadyExists(err) {
			existing := &syngit.RemoteUserBinding{}
			if getErr := r.Get(ctx, types.NamespacedName{Name: name, Namespace: remoteUser.Namespace}, existing); getErr != nil {
				return nil, getErr
			}
			return existing, nil
		}
		return nil, err
	}

	return rub, nil
}

// ensureRemoteUserRef ensures the RemoteUser is in the RUB's remoteUserRefs. Returns true if modified.
func (r *AssociationPolicyReconciler) ensureRemoteUserRef(ctx context.Context, rub *syngit.RemoteUserBinding, remoteUserName string) error {
	for _, ref := range rub.Spec.RemoteUserRefs {
		if ref.Name == remoteUserName {
			return nil
		}
	}
	if err := r.Get(ctx, types.NamespacedName{Name: rub.Name, Namespace: rub.Namespace}, rub); err != nil {
		return err
	}
	rub.Spec.RemoteUserRefs = append(rub.Spec.RemoteUserRefs, corev1.ObjectReference{Name: remoteUserName})

	return r.Update(ctx, rub)
}

// associateRemoteTargets finds all one-or-many-branches RemoteTargets and ensures they're in the RUB.
func (r *AssociationPolicyReconciler) associateExistingRemoteTargets(ctx context.Context, rub *syngit.RemoteUserBinding) error {
	rtList := &syngit.RemoteTargetList{}
	listOps := &client.ListOptions{
		Namespace: rub.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.RtLabelKeyPolicy:  syngit.RtLabelValueOneOrManyBranches,
		}),
	}
	if err := r.List(ctx, rtList, listOps); err != nil {
		return err
	}

	spec := *rub.Spec.DeepCopy()
	modified := false
	for _, rt := range rtList.Items {
		found := false
		for _, ref := range spec.RemoteTargetRefs {
			if ref.Name == rt.Name {
				found = true
				break
			}
		}
		if !found {
			spec.RemoteTargetRefs = append(spec.RemoteTargetRefs, corev1.ObjectReference{Name: rt.Name})
			modified = true
		}
	}

	if !modified {
		return nil
	}
	return utils.UpdateOrDeleteManagedRemoteUserBinding(ctx, r.Client, spec, *rub)
}

// cleanupAssociation removes the RemoteUser from its managed RUB and deletes the RUB if empty.
func (r *AssociationPolicyReconciler) cleanupAssociation(ctx context.Context, remoteUser *syngit.RemoteUser, sanitizedUsername string) error {
	if sanitizedUsername == "" {
		return nil
	}

	rubList := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.K8sUserLabelKey:   sanitizedUsername,
		}),
		Namespace: remoteUser.Namespace,
	}
	if err := r.List(ctx, rubList, listOps); err != nil {
		return err
	}

	for _, rub := range rubList.Items {
		newRefs := []corev1.ObjectReference{}
		for _, ref := range rub.Spec.RemoteUserRefs {
			if ref.Name != remoteUser.Name {
				newRefs = append(newRefs, ref)
			}
		}
		if len(newRefs) == len(rub.Spec.RemoteUserRefs) {
			continue
		}

		spec := *rub.Spec.DeepCopy()
		spec.RemoteUserRefs = newRefs
		if err := utils.UpdateOrDeleteManagedRemoteUserBinding(ctx, r.Client, spec, rub); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
	}

	return nil
}

// generateUniqueName generates a unique name by appending a numeric suffix if needed.
func (r *AssociationPolicyReconciler) generateUniqueName(ctx context.Context, baseName, namespace string) (string, error) {
	name := baseName
	for i := 0; i < 100; i++ {
		if i > 0 {
			name = fmt.Sprintf("%s-%d", baseName, i)
		}
		existing := &syngit.RemoteUserBinding{}
		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existing)
		if apierrors.IsNotFound(err) {
			return name, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("could not generate unique name for %s", baseName)
}

// findRemoteUsersForRemoteTarget maps RemoteTarget changes to RemoteUser reconcile requests.
func (r *AssociationPolicyReconciler) findRemoteUsersForRemoteTarget(ctx context.Context, obj client.Object) []reconcile.Request {
	rt, ok := obj.(*syngit.RemoteTarget)
	if !ok {
		return nil
	}

	// Only care about managed one-or-many-branches RemoteTargets
	if rt.Labels[syngit.RtLabelKeyPolicy] != syngit.RtLabelValueOneOrManyBranches {
		return nil
	}

	// Find all RemoteUsers with managed annotation in the same namespace
	ruList := &syngit.RemoteUserList{}
	if err := r.List(ctx, ruList, &client.ListOptions{Namespace: rt.Namespace}); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, ru := range ruList.Items {
		if ru.Annotations[syngit.RubAnnotationKeyManaged] == "true" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ru.Name,
					Namespace: ru.Namespace,
				},
			})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *AssociationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&syngit.RemoteUser{}).
		Watches(
			&syngit.RemoteTarget{},
			handler.EnqueueRequestsFromMapFunc(r.findRemoteUsersForRemoteTarget),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Named("association-policy").
		Complete(r)
}
