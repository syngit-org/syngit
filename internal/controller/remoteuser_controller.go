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
	"os"

	syngit "github.com/syngit-org/syngit/api/v1beta2"
	"github.com/syngit-org/syngit/internal/utils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
)

// RemoteUserReconciler reconciles a RemoteUser object
type RemoteUserReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Namespace string
}

// +kubebuilder:rbac:groups=syngit.io,resources=remoteusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syngit.io,resources=remoteusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=syngit.io,resources=remoteusers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *RemoteUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// Get the RemoteUser Object
	var remoteUser syngit.RemoteUser
	if err := r.Get(ctx, req.NamespacedName, &remoteUser); err != nil {
		// does not exists -> deleted
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Log.Info("Reconcile request",
		"resource", "remoteuser",
		"namespace", remoteUser.Namespace,
		"name", remoteUser.Name,
	)

	condition := &v1.Condition{
		LastTransitionTime: v1.Now(),
		Type:               "SecretBound",
		Status:             v1.ConditionFalse,
	}

	// Get the referenced Secret
	var secret corev1.Secret
	namespacedNameSecret := types.NamespacedName{Namespace: req.Namespace, Name: remoteUser.Spec.SecretRef.Name}
	if err := r.Get(ctx, namespacedNameSecret, &secret); err != nil {
		remoteUser.Status.SecretBoundStatus = syngit.SecretNotFound
		remoteUser.Status.ConnexionStatus.Status = ""

		condition.Reason = "SecretNotFound"
		condition.Status = v1.ConditionFalse
		condition.Message = string(syngit.SecretNotFound)
		_ = r.updateStatus(ctx, &remoteUser, *condition)

		return ctrl.Result{}, nil
	}

	remoteUser.Status.SecretBoundStatus = syngit.SecretFound
	condition.Reason = "SecretFound"
	condition.Message = string(syngit.SecretFound)

	// Check if the referenced Secret is a basic-auth type
	if secret.Type != corev1.SecretTypeBasicAuth {

		remoteUser.Status.SecretBoundStatus = syngit.SecretWrongType

		condition.Reason = "SecretWrongType"
		condition.Message = string(syngit.SecretWrongType)
		_ = r.updateStatus(ctx, &remoteUser, *condition)

		return ctrl.Result{}, nil
	}

	remoteUser.Status.SecretBoundStatus = syngit.SecretBound
	condition.Message = string(syngit.SecretBound)
	condition.Type = "SecretBound"
	condition.Reason = "SecretBound"
	condition.Status = v1.ConditionTrue

	// Update the status of RemoteUser
	_ = r.updateStatus(ctx, &remoteUser, *condition)

	return ctrl.Result{}, nil
}

func (r *RemoteUserReconciler) updateStatus(ctx context.Context, remoteUser *syngit.RemoteUser, condition v1.Condition) error {
	conditions := utils.TypeBasedConditionUpdater(remoteUser.Status.DeepCopy().Conditions, condition)

	remoteUser.Status.Conditions = conditions
	if err := r.Status().Update(ctx, remoteUser); err != nil {
		return err
	}
	return nil
}

func (r *RemoteUserReconciler) findObjectsForSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	attachedRemoteUsers := &syngit.RemoteUserList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(syngit.SecretRefField, secret.GetName()),
		Namespace:     secret.GetNamespace(),
	}
	err := r.List(ctx, attachedRemoteUsers, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(attachedRemoteUsers.Items))
	for i, item := range attachedRemoteUsers.Items {
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
func (r *RemoteUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &syngit.RemoteUser{}, syngit.SecretRefField, func(rawObj client.Object) []string {
		// Extract the Secret name from the RemoteUser Spec, if one is provided
		remoteUser := rawObj.(*syngit.RemoteUser)
		if remoteUser.Spec.SecretRef.Name == "" {
			return nil
		}
		return []string{remoteUser.Spec.SecretRef.Name}
	}); err != nil {
		return err
	}

	// Recorder to manage events
	recorder := mgr.GetEventRecorderFor("remoteuser-controller")
	r.Recorder = recorder

	managerNamespace := os.Getenv("MANAGER_NAMESPACE")
	r.Namespace = managerNamespace

	return ctrl.NewControllerManagedBy(mgr).
		For(&syngit.RemoteUser{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSecret),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
