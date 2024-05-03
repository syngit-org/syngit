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
	"k8s.io/apimachinery/pkg/fields"
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
	Scheme        *runtime.Scheme
	webhookServer WebhookInterceptsAll
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
		r.webhookServer.DestroyPathHandler(req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	rINamespace := resourcesInterceptor.Namespace
	rIBName := resourcesInterceptor.Name

	var prefixMsg = "[" + rINamespace + "/" + rIBName + "]" + tabString

	log.Log.Info(prefixMsg + "Reconciling request received")

	// For each included resources, Reset the handler related to this object
	r.webhookServer.DestroyPathHandler(req.NamespacedName)
	r.webhookServer.CreatePathHandler(resourcesInterceptor)

	// TODO Make a loop on included

	// // Define your ValidatingWebhook object
	// webhookName := "your-webhook-name"
	// serviceName := "your-service-name"
	// namespace := "your-namespace"
	// var scope admissionv1.ScopeType = admissionv1.NamespacedScope

	// // Create a new ValidatingWebhook object
	// newWebhook := &admissionv1.ValidatingWebhook{
	// 	Name:                    webhookName,
	// 	AdmissionReviewVersions: []string{"v1"},
	// 	Rules: []admissionv1.RuleWithOperations{
	// 		{
	// 			Operations: []admissionv1.OperationType{"CREATE", "UPDATE", "DELETE"},
	// 			Rule: admissionv1.Rule{
	// 				APIGroups:   []string{""},
	// 				APIVersions: []string{"v1"},
	// 				Resources:   []string{"pods"},
	// 				Scope:       &scope,
	// 			},
	// 		},
	// 	},
	// 	ClientConfig: admissionv1.WebhookClientConfig{
	// 		Service: &admissionv1.ServiceReference{
	// 			Name:      serviceName,
	// 			Namespace: namespace,
	// 			Path:      &webhookPath,
	// 		},
	// 	},
	// 	FailurePolicy:     failurePolicy,
	// 	NamespaceSelector: rINamespace,
	// }

	// ctrl.SetControllerReference(owner, )

	return ctrl.Result{}, nil
}

func KindToResource(kind string) (schema.GroupVersionResource, error) {
	gvk, _ := schema.ParseKindArg(kind)
	return gvk.GroupVersion().WithResource(fmt.Sprintf("%s/%s", gvk.Group, gvk.Kind)), nil
}

func parsegvkList(gvkGivenList []kgiov1.NamespaceScopedKinds) []kgiov1.GroupVersionKindName {
	var gvkList []kgiov1.GroupVersionKindName

	for _, gvkGiven := range gvkGivenList {
		for _, group := range gvkGiven.APIGroups {
			for _, version := range gvkGiven.APIVersions {
				for _, kind := range gvkGiven.Kinds {
					gvkn := kgiov1.GroupVersionKindName{
						GroupVersionKind: &schema.GroupVersionKind{
							Group:   group,
							Version: version,
							Kind:    kind,
						},
					}
					if len(gvkGiven.Names) != 0 {
						for _, name := range gvkGiven.Names {
							gvkn.Name = name
						}
					}
					gvkList = append(gvkList, gvkn)
				}
			}
		}
	}

	return gvkList
}

func (r *ResourcesInterceptorReconciler) dynamicObjectFinder(ctx context.Context, obj client.Object) []reconcile.Request {
	attachedResourcesInterceptor := &kgiov1.ResourcesInterceptorList{}

	group := obj.GetObjectKind().GroupVersionKind().Group
	version := obj.GetObjectKind().GroupVersionKind().Version
	kind := obj.GetObjectKind().GroupVersionKind().Kind

	// Search by GVK + name
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(includedResourcesField, group+"/"+version+"/"+kind+"/"+obj.GetName()),
		Namespace:     obj.GetNamespace(),
	}
	err := r.List(ctx, attachedResourcesInterceptor, listOps)
	if err != nil {
		fmt.Println(err)
		return []reconcile.Request{}
	}
	fmt.Println(len(attachedResourcesInterceptor.Items))

	if len(attachedResourcesInterceptor.Items) == 0 {
		// Search by GVK only
		listOps = &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(includedResourcesField, group+"/"+version+"/"+kind+"/"),
			Namespace:     obj.GetNamespace(),
		}
		attachedResourcesInterceptor = &kgiov1.ResourcesInterceptorList{}
		err := r.List(ctx, attachedResourcesInterceptor, listOps)
		if err != nil {
			fmt.Println(err)
			return []reconcile.Request{}
		}
		fmt.Println(len(attachedResourcesInterceptor.Items))
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
	nameField              = ".metadata.name"
)

// SetupWithManager sets up the controller with the Manager.
func (r *ResourcesInterceptorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &kgiov1.ResourcesInterceptor{}, includedResourcesField, func(rawObj client.Object) []string {

		resourcesInterceptor := rawObj.(*kgiov1.ResourcesInterceptor)
		gvksRefName := []string{}

		gvks := parsegvkList(resourcesInterceptor.Spec.IncludedResources)
		if len(gvks) == 0 {
			return nil
		}

		for _, gvkn := range gvks {
			gvksRefName = append(gvksRefName, gvkn.GroupVersionKind.Group+"/"+gvkn.GroupVersionKind.Version+"/"+gvkn.GroupVersionKind.Kind+"/"+gvkn.Name)
		}

		return gvksRefName
	}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &kgiov1.ResourcesInterceptor{}, nameField, func(rawObj client.Object) []string {

		resourcesInterceptor := rawObj.(*kgiov1.ResourcesInterceptor)

		return []string{resourcesInterceptor.Name}
	}); err != nil {
		return err
	}

	// Initialize the webhookServer
	r.webhookServer = WebhookInterceptsAll{}
	r.webhookServer.Start()

	return ctrl.NewControllerManagedBy(mgr).
		For(&kgiov1.ResourcesInterceptor{}).
		Watches(
			&kgiov1.ResourcesInterceptor{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {

				attachedResourcesInterceptor := &kgiov1.ResourcesInterceptorList{}
				listOps := &client.ListOptions{
					FieldSelector: fields.OneTermEqualSelector(nameField, obj.GetName()),
					Namespace:     obj.GetNamespace(),
				}
				err := r.List(ctx, attachedResourcesInterceptor, listOps)
				if err != nil {
					return []reconcile.Request{}
				}

				for _, resourcesInterceptor := range attachedResourcesInterceptor.Items {
					ri := obj.(*kgiov1.ResourcesInterceptor)
					dynamicResource := &unstructured.Unstructured{}

					// Filter ONLY the resources we want to watch for -> defined in the spec of the ResourcesInterceptor
					for _, gvk := range parsegvkList(resourcesInterceptor.Spec.IncludedResources) {
						dynamicResource.SetGroupVersionKind(*gvk.GroupVersionKind)

						err := ctrl.NewControllerManagedBy(mgr).
							For(ri).
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
				}

				return []reconcile.Request{}
			}),
		).
		Complete(r)
}
