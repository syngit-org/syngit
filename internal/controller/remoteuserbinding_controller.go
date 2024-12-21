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

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	syngit "github.com/syngit-org/syngit/api/v1beta2"
)

// RemoteUserBindingReconciler reconciles a RemoteUserBinding object
type RemoteUserBindingReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=syngit.io,resources=remoteuserbindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=syngit.io,resources=remoteuserbindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=syngit.io,resources=remoteuserbindings/finalizers,verbs=update

func (r *RemoteUserBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// Get the RemoteUserBinding Object
	var remoteUserBinding syngit.RemoteUserBinding
	if err := r.Get(ctx, req.NamespacedName, &remoteUserBinding); err != nil {
		// does not exists -> deleted
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	rUBNamespace := remoteUserBinding.Namespace
	rUBName := remoteUserBinding.Name
	subject := remoteUserBinding.Spec.Subject

	var prefixMsg = "[" + rUBNamespace + "/" + rUBName + "]"
	log.Log.Info(prefixMsg + " Reconciling request received")

	// Get the referenced RemoteUsers
	var isGloballyBound bool = false
	var isGloballyNotBound bool = false

	gitUserHosts := []syngit.GitUserHost{}
	for _, remoteUserRef := range remoteUserBinding.Spec.RemoteRefs {

		// Set already known values about this RemoteUser
		var gitUserHost syngit.GitUserHost = syngit.GitUserHost{}
		gitUserHost.State = syngit.NotBound
		gitUserHost.RemoteUserUsed = remoteUserRef.Name

		var remoteUser syngit.RemoteUser
		retrievedRemoteUser := types.NamespacedName{Namespace: req.Namespace, Name: remoteUserRef.Name}

		// Get the concerned RemoteUser
		if err := r.Get(ctx, retrievedRemoteUser, &remoteUser); err != nil {
			r.Recorder.Event(&remoteUserBinding, "Warning", "NotBound", gitUserHost.RemoteUserUsed+" not bound")
			isGloballyNotBound = true
		} else {
			gitUserHost.GitFQDN = remoteUser.Spec.GitBaseDomainFQDN
			gitUserHost.SecretRef = remoteUser.Spec.SecretRef
			gitUserHost.State = syngit.Bound
			r.Recorder.Event(&remoteUserBinding, "Normal", "Bound", gitUserHost.RemoteUserUsed+" bound")
			isGloballyBound = true
		}

		gitUserHosts = append(gitUserHosts, gitUserHost)

	}
	remoteUserBinding.Status.GitUserHosts = gitUserHosts

	if isGloballyBound && isGloballyNotBound {
		remoteUserBinding.Status.GlobalState = syngit.PartiallyBound
		r.Recorder.Event(&remoteUserBinding, "Warning", "PartiallyBound", "Some of the git repos are not bound")
	} else {
		if isGloballyBound {
			remoteUserBinding.Status.GlobalState = syngit.Bound
			r.Recorder.Event(&remoteUserBinding, "Normal", "Bound", "Every git repos are bound")
		} else {
			remoteUserBinding.Status.GlobalState = syngit.NotBound
			r.Recorder.Event(&remoteUserBinding, "Warning", "NotBound", "None of the git repos are bound")
		}
	}

	remoteUserBinding.Status.UserKubernetesID = subject.Name

	if err := r.Status().Update(ctx, &remoteUserBinding); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}

func (r *RemoteUserBindingReconciler) findObjectsForRemoteUser(ctx context.Context, remoteUser client.Object) []reconcile.Request {
	attachedRemoteUserBindings := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(remoteRefsField, remoteUser.GetName()),
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

const (
	remoteRefsField = ".spec.remoteRefs"
)

// SetupWithManager sets up the controller with the Manager.
func (r *RemoteUserBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &syngit.RemoteUserBinding{}, remoteRefsField, func(rawObj client.Object) []string {

		remoteUserRefsName := []string{}

		remoteUserBinding := rawObj.(*syngit.RemoteUserBinding)
		if len(remoteUserBinding.Spec.RemoteRefs) == 0 {
			return nil
		}
		for _, remoteUserRef := range remoteUserBinding.Spec.RemoteRefs {
			if remoteUserRef.Name == "" {
				return nil
			}
			remoteUserRefsName = append(remoteUserRefsName, remoteUserRef.DeepCopy().Name)
		}
		return remoteUserRefsName
	}); err != nil {
		return err
	}
	recorder := mgr.GetEventRecorderFor("remoteuserbinding-controller")
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
