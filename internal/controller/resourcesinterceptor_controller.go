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
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
//+kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch

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

func parsegvkList(gvkGivenList []kgiov1.NamespaceScopedKinds) []schema.GroupVersionKind {
	var gvkList []schema.GroupVersionKind

	for _, gvkGiven := range gvkGivenList {
		for _, group := range gvkGiven.APIGroups {
			for _, version := range gvkGiven.APIVersions {
				for _, kind := range gvkGiven.Kinds {
					gvk := schema.GroupVersionKind{
						Group:   group,
						Version: version,
						Kind:    kind,
					}
					gvkList = append(gvkList, gvk)
				}
			}
		}
	}

	return gvkList
}

func (r *ResourcesInterceptorReconciler) dynamicObjectFinder(ctx context.Context, obj client.Object) []reconcile.Request {
	attachedResourcesInterceptor := &kgiov1.ResourcesInterceptorList{}
	// The cache only supports exact matching
	// Since we cant to watch ALL the resources with this GVK, we cannot exact match a specific resource
	// listOps := &client.ListOptions{
	// 	FieldSelector: fields.Everything(),
	// 	Namespace:     obj.GetNamespace(),
	// }

	// Everything because we already filtered inside the top-level Watches function
	err := r.List(ctx, attachedResourcesInterceptor)
	if err != nil {
		fmt.Println(err)
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(attachedResourcesInterceptor.Items))
	for i, item := range attachedResourcesInterceptor.Items {
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
	includedResourcesField = ".spec.includedResources"
	excludedResourcesField = ".spec.excludedResources"
)

// SetupWithManager sets up the controller with the Manager.
func (r *ResourcesInterceptorReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&kgiov1.ResourcesInterceptor{}).
		Watches(
			&kgiov1.ResourcesInterceptor{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {

				resourcesInterceptor := obj.(*kgiov1.ResourcesInterceptor)
				dynamicResource := &unstructured.Unstructured{}

				// Filter ONLY the resources we want to watch for -> defined in the spec of the ResourcesInterceptor
				for _, gvk := range parsegvkList(resourcesInterceptor.Spec.IncludedResources) {
					dynamicResource.SetGroupVersionKind(gvk)

					err := ctrl.NewControllerManagedBy(mgr).
						For(&kgiov1.ResourcesInterceptor{}).
						Watches(
							dynamicResource,
							handler.EnqueueRequestsFromMapFunc(r.dynamicObjectFinder),
							builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
						).
						WithLogConstructor(func(request *reconcile.Request) logr.Logger {
							return mgr.GetLogger()
						}).
						Complete(r)
					if err != nil {
						mgr.GetLogger().Error(err, fmt.Sprintf("Unable to create a dynamic controller for %s", gvk))
					}
				}

				return []reconcile.Request{}
			}),
		).
		Complete(r)
}
