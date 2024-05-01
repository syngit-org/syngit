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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kgiov1 "dams.kgio/kgio/api/v1"
)

// ResourcesInterceptorReconciler reconciles a ResourcesInterceptor object
type ResourcesInterceptorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=kgio.dams.kgio,resources=resourcesinterceptors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kgio.dams.kgio,resources=resourcesinterceptors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kgio.dams.kgio,resources=resourcesinterceptors/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ResourcesInterceptor object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *ResourcesInterceptorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	var tabString = "\n 					"

	// Get the ResourcesInterceptor Object
	var resourcesInterceptor kgiov1.ResourcesInterceptor
	if err := r.Get(ctx, req.NamespacedName, &resourcesInterceptor); err != nil {
		// does not exists -> deleted
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	rINamespace := resourcesInterceptor.Namespace
	rIBName := resourcesInterceptor.Name

	var prefixMsg = "[" + rINamespace + "/" + rIBName + "]" + tabString

	log.Log.Info(prefixMsg + "Reconciling request received")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourcesInterceptorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kgiov1.ResourcesInterceptor{}).
		Complete(r)
}
