/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// RemoteUserBindingReconciler reconciles a RemoteUserBinding object
type RemoteUserBindingReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=syngit.io,resources=remoteuserbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syngit.io,resources=remoteuserbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=syngit.io,resources=remoteuserbindings/finalizers,verbs=update

func (r *RemoteUserBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// Get the RemoteUserBinding Object
	var remoteUserBinding syngit.RemoteUserBinding
	if err := r.Get(ctx, req.NamespacedName, &remoteUserBinding); err != nil {
		// does not exists -> deleted
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	subject := remoteUserBinding.Spec.Subject

	log.Log.Info("Reconcile request",
		"resource", "remoteuserbinding",
		"namespace", remoteUserBinding.Namespace,
		"name", remoteUserBinding.Name,
	)

	isManaged := remoteUserBinding.Labels[syngit.ManagedByLabelKey] == syngit.ManagedByLabelValue

	// Get the referenced RemoteUsers
	var isGloballyBound = true

	gitUserHosts := []syngit.RemoteUserHost{}
	missingRefs := []string{}
	for _, remoteUserRef := range remoteUserBinding.Spec.RemoteUserRefs {

		// Set already known values about this RemoteUser
		var gitUserHost = syngit.RemoteUserHost{}
		gitUserHost.RemoteUserUsed = remoteUserRef.Name

		var remoteUser syngit.RemoteUser
		retrievedRemoteUser := types.NamespacedName{Namespace: req.Namespace, Name: remoteUserRef.Name}

		// Get the concerned RemoteUser
		if err := r.Get(ctx, retrievedRemoteUser, &remoteUser); err != nil {
			missingRefs = append(missingRefs, remoteUserRef.Name)
			gitUserHost.State = syngit.NotBound
			r.Recorder.Eventf(&remoteUserBinding, nil, "Warning", "NotBound", gitUserHost.RemoteUserUsed+" not bound", "")
			isGloballyBound = false
		} else {
			gitUserHost.GitFQDN = remoteUser.Spec.GitBaseDomainFQDN
			gitUserHost.SecretRef = remoteUser.Spec.SecretRef
			gitUserHost.State = syngit.Bound
			r.Recorder.Eventf(&remoteUserBinding, nil, "Normal", "Bound", gitUserHost.RemoteUserUsed+" bound", "")
		}

		gitUserHosts = append(gitUserHosts, gitUserHost)

	}

	// For managed RUBs this controller is the leveler: prune refs to RemoteUsers
	// that no longer exist. This makes the association self-healing. If a stale
	// reconcile re-added a ref to a since-deleted RemoteUser, it is removed here
	// on the next pass (this controller is re-triggered on RemoteUser deletes via
	// findObjectsForRemoteUser). It is safe because all controllers share one
	// informer cache, so a ref only ever reads as missing once its RemoteUser is
	// genuinely gone, never for a freshly-created one. Unmanaged RUBs are left
	// untouched: the user listed those refs explicitly, so they stay NotBound.
	if isManaged && len(missingRefs) > 0 {
		missing := make(map[string]bool, len(missingRefs))
		for _, name := range missingRefs {
			missing[name] = true
		}
		if err := utils.MutateOrDeleteManagedRemoteUserBinding(ctx, r.Client, req.NamespacedName,
			func(fresh *syngit.RemoteUserBinding) error {
				kept := make([]corev1.ObjectReference, 0, len(fresh.Spec.RemoteUserRefs))
				for _, ref := range fresh.Spec.RemoteUserRefs {
					if !missing[ref.Name] {
						kept = append(kept, ref)
					}
				}
				fresh.Spec.RemoteUserRefs = kept
				return nil
			}); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		// The spec change (or RUB deletion when it becomes empty) re-triggers
		// this controller, which recomputes status from the pruned set.
		return ctrl.Result{}, nil
	}

	remoteUserBinding.Status.RemoteUserHosts = gitUserHosts

	if !isGloballyBound {
		remoteUserBinding.Status.RemoteUserState = syngit.PartiallyBound
		r.Recorder.Eventf(&remoteUserBinding, nil, "Warning", "PartiallyBound", partiallyBoundMessage, "")
	} else {
		remoteUserBinding.Status.RemoteUserState = syngit.Bound
		r.Recorder.Eventf(&remoteUserBinding, nil, "Normal", "Bound", boundMessage, "")
	}

	remoteUserBinding.Status.UserKubernetesID = subject.Name

	_ = r.Status().Update(ctx, &remoteUserBinding)
	return ctrl.Result{}, nil
}

func (r *RemoteUserBindingReconciler) findObjectsForRemoteUser(ctx context.Context, remoteUser client.Object) []reconcile.Request {
	attachedRemoteUserBindings := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(syngit.RemoteRefsField, remoteUser.GetName()),
		Namespace:     remoteUser.GetNamespace(),
	}
	err := r.List(ctx, attachedRemoteUserBindings, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(attachedRemoteUserBindings.Items))
	for i, item := range attachedRemoteUserBindings.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemoteUserBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &syngit.RemoteUserBinding{}, syngit.RemoteRefsField, func(rawObj client.Object) []string {

		remoteUserRefsName := []string{}

		remoteUserBinding := rawObj.(*syngit.RemoteUserBinding)
		if len(remoteUserBinding.Spec.RemoteUserRefs) == 0 {
			return nil
		}
		for _, remoteUserRef := range remoteUserBinding.Spec.RemoteUserRefs {
			if remoteUserRef.Name == "" {
				return nil
			}
			remoteUserRefsName = append(remoteUserRefsName, remoteUserRef.DeepCopy().Name)
		}
		return remoteUserRefsName
	}); err != nil {
		return err
	}
	recorder := mgr.GetEventRecorder("remoteuserbinding-controller")
	r.Recorder = recorder

	return ctrl.NewControllerManagedBy(mgr).
		For(&syngit.RemoteUserBinding{}).
		Watches(
			&syngit.RemoteUser{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForRemoteUser),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
