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

	kgiov1 "dams.kgio/kgio/api/v1"
)

// GitUserBindingReconciler reconciles a GitUserBinding object
type GitUserBindingReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=kgio.dams.kgio,resources=gituserbindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kgio.dams.kgio,resources=gituserbindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kgio.dams.kgio,resources=gituserbindings/finalizers,verbs=update
//+kubebuilder:rbac:groups=kgio.dams.kgio,resources=gitremote,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GitUserBinding object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *GitUserBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// Get the GitUserBinding Object
	var gitUserBinding kgiov1.GitUserBinding
	if err := r.Get(ctx, req.NamespacedName, &gitUserBinding); err != nil {
		// does not exists -> deleted
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	gUBNamespace := gitUserBinding.Namespace
	gUBName := gitUserBinding.Name
	subject := gitUserBinding.Spec.Subject

	var prefixMsg = "[" + gUBNamespace + "/" + gUBName + "]"
	log.Log.Info(prefixMsg + " Reconciling request received")

	// Get the referenced GitRemotes
	var isGloballyBound bool = false
	var isGloballyNotBound bool = false

	var gitUserHosts []kgiov1.GitUserHost
	for _, gitRemoteRef := range gitUserBinding.Spec.RemoteRefs {

		// Set already known values about this GitRemote
		var gitUserHost kgiov1.GitUserHost = kgiov1.GitUserHost{}
		gitUserHost.State = kgiov1.NotBound
		gitUserHost.GitRemoteUsed = gitRemoteRef.Name

		var gitRemote kgiov1.GitRemote
		retrievedGitRemote := types.NamespacedName{Namespace: req.Namespace, Name: gitRemoteRef.Name}

		// Get the concerned GitRemote
		if err := r.Get(ctx, retrievedGitRemote, &gitRemote); err != nil {
			r.Recorder.Event(&gitUserBinding, "Warning", "NotBound", gitUserHost.GitRemoteUsed+" not bound")
			isGloballyNotBound = true
		} else {
			gitUserHost.GitFQDN = gitRemote.Spec.GitBaseDomainFQDN
			gitUserHost.SecretRef = gitRemote.Spec.SecretRef
			gitUserHost.State = kgiov1.Bound
			r.Recorder.Event(&gitUserBinding, "Normal", "Bound", gitUserHost.GitRemoteUsed+" bound")
			isGloballyBound = true
		}

		gitUserHosts = append(gitUserHosts, gitUserHost)

	}
	gitUserBinding.Status.GitUserHosts = gitUserHosts

	if isGloballyBound && isGloballyNotBound {
		gitUserBinding.Status.GlobalState = kgiov1.PartiallyBound
		r.Recorder.Event(&gitUserBinding, "Warning", "PartiallyBound", "Some of the git repos are not bound")
	} else {
		if isGloballyBound {
			gitUserBinding.Status.GlobalState = kgiov1.Bound
			r.Recorder.Event(&gitUserBinding, "Normal", "Bound", "Every git repos are bound")
		} else {
			gitUserBinding.Status.GlobalState = kgiov1.NotBound
			r.Recorder.Event(&gitUserBinding, "Warning", "NotBound", "None of the git repos are bound")
		}
	}

	gitUserBinding.Status.UserKubernetesID = subject.Name

	if err := r.Status().Update(ctx, &gitUserBinding); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}

func (r *GitUserBindingReconciler) findObjectsForGitRemote(ctx context.Context, gitRemote client.Object) []reconcile.Request {
	attachedGitUserBindings := &kgiov1.GitUserBindingList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(remoteRefsField, gitRemote.GetName()),
		Namespace:     gitRemote.GetNamespace(),
	}
	err := r.List(ctx, attachedGitUserBindings, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(attachedGitUserBindings.Items))
	for i, item := range attachedGitUserBindings.Items {
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
func (r *GitUserBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &kgiov1.GitUserBinding{}, remoteRefsField, func(rawObj client.Object) []string {

		gitRemoteRefsName := []string{}

		gitUserBinding := rawObj.(*kgiov1.GitUserBinding)
		if len(gitUserBinding.Spec.RemoteRefs) == 0 {
			return nil
		}
		for _, gitRemoteRef := range gitUserBinding.Spec.RemoteRefs {
			if gitRemoteRef.Name == "" {
				return nil
			}
			gitRemoteRefsName = append(gitRemoteRefsName, gitRemoteRef.DeepCopy().Name)
		}
		return gitRemoteRefsName
	}); err != nil {
		return err
	}
	recorder := mgr.GetEventRecorderFor("gituserbinding-controller")
	r.Recorder = recorder

	return ctrl.NewControllerManagedBy(mgr).
		For(&kgiov1.GitUserBinding{}).
		Watches(
			&kgiov1.GitRemote{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForGitRemote),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
