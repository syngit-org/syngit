/*
Copyright 2026.

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
	"errors"
	"fmt"

	interceptor "github.com/syngit-org/syngit/internal/interceptor"
	"github.com/syngit-org/syngit/internal/policy"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"github.com/syngit-org/syngit/pkg/kube"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// ClusterWideRemoteSyncerReconciler reconciles a ClusterWideRemoteSyncer object.
// It is the sibling of RemoteSyncerReconciler: same responsibilities, but the
// namespaces it intercepts come from spec.namespaceSelector instead of the
// object's own namespace, and its identities come from
// spec.identityStoreNamespace.
type ClusterWideRemoteSyncerReconciler struct {
	client.Client
	// Owns this syncer's entry in the shared dynamic webhook configuration.
	dynamicWebhookManager

	Scheme *runtime.Scheme
	// WebhookServer is the interception server shared with the RemoteSyncer
	// controller. Required.
	WebhookServer *interceptor.WebhookInterceptsAll
	Namespace     string
	Recorder      events.EventRecorder

	branchTargetPolicy *policy.BranchTargetPolicy
	userSpecificPolicy *policy.UserSpecificPolicy
}

// +kubebuilder:rbac:groups=syngit.io,resources=clusterwideremotesyncers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syngit.io,resources=clusterwideremotesyncers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=syngit.io,resources=clusterwideremotesyncers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch

func (r *ClusterWideRemoteSyncerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Core: reconcile the dynamic webhook configuration. This also handles the
	// already-deleted case (Get returns NotFound) by removing the syncer's
	// webhook entry.
	coreResult, coreErr := r.reconcileWebhook(ctx, req)

	// Policies run only while the object still exists. Once it is fully gone
	// (NotFound), its finalizers are already removed, so there is nothing to do.
	var cwrs syngit.ClusterWideRemoteSyncer
	if err := r.Get(ctx, req.NamespacedName, &cwrs); err != nil {
		return coreResult, coreErr
	}

	polResult, polErr := policy.RunPolicies[syngit.Syncer](ctx, r.Client, &cwrs,
		[]policy.Policy[syngit.Syncer]{r.branchTargetPolicy, r.userSpecificPolicy})

	return kube.MergeResults(coreResult, polResult), errors.Join(coreErr, polErr)
}

// reconcileWebhook manages this syncer's entry in the shared dynamic
// ValidatingWebhookConfiguration (and removes it when the object is gone).
func (r *ClusterWideRemoteSyncerReconciler) reconcileWebhook(ctx context.Context, req ctrl.Request) (ctrl.Result, error) { // nolint:unparam
	_ = log.FromContext(ctx)

	webhookPath := interceptor.ClusterWideRemoteSyncerWebhookPath(req.Name)

	isDeleted := false
	var cwrs syngit.ClusterWideRemoteSyncer
	if err := r.Get(ctx, req.NamespacedName, &cwrs); err != nil {
		// does not exist -> deleted
		r.WebhookServer.Unregister(webhookPath)
		isDeleted = true
	} else {
		r.WebhookServer.RegisterClusterWide(cwrs, webhookPath)
	}

	log.Log.Info("Reconcile request",
		"resource", "clusterwideremotesyncer",
		"name", req.Name,
	)

	entry := dynamicWebhookEntry{
		// Kept distinct from the "<name>.<namespace>.syngit.io" of a namespaced
		// RemoteSyncer so the two controllers can never claim the same entry.
		name:  req.Name + ".clusterwide.syngit.io",
		path:  webhookPath,
		rules: cwrs.Spec.ScopedResources.Rules,
		// Passed through verbatim: a nil selector matches every namespace, which
		// is exactly the documented meaning of leaving the field unset.
		namespaceSelector: cwrs.Spec.NamespaceSelector,
		objectSelector:    cwrs.Spec.ScopedResources.ObjectSelector,
	}

	condition := &v1.Condition{
		LastTransitionTime: v1.Now(),
		Type:               "WebhookReconciled",
		Status:             v1.ConditionFalse,
	}

	if err := r.upsert(ctx, entry, isDeleted); err != nil {
		r.Recorder.Eventf(&cwrs, nil, "Warning", "WebhookNotUpdated", "The dynamic webhook has not been updated", "")

		condition.Reason = "WebhookNotUpdated"
		condition.Message = "The dynamic webhook has not been updated: " + err.Error()
		_ = r.updateStatus(ctx, &cwrs, *condition)

		return reconcile.Result{}, err
	}

	if isDeleted {
		return ctrl.Result{}, nil
	}

	condition.Reason = "WebhookUpdated"
	condition.Message = "The resources have been successfully assigned to the webhook"
	condition.Status = v1.ConditionTrue
	_ = r.updateStatus(ctx, &cwrs, *condition)

	return ctrl.Result{}, nil
}

func (r *ClusterWideRemoteSyncerReconciler) updateStatus(ctx context.Context, cwrs *syngit.ClusterWideRemoteSyncer, condition v1.Condition) error {
	conditions := kube.SetCondition(cwrs.Status.DeepCopy().Conditions, condition)

	cwrs.Status.Conditions = conditions
	return r.Status().Update(ctx, cwrs)
}

// findObjectsForDynamicWebhook re-enqueues every ClusterWideRemoteSyncer when the
// shared configuration changes, so an entry deleted out from under us is restored.
func (r *ClusterWideRemoteSyncerReconciler) findObjectsForDynamicWebhook(ctx context.Context, webhook client.Object) []reconcile.Request {
	cwrsList := &syngit.ClusterWideRemoteSyncerList{}
	if err := r.List(ctx, cwrsList); err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(cwrsList.Items))
	for i, item := range cwrsList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{Name: item.GetName()},
		}
	}
	return requests
}

// findSyncersForRUB maps a managed RemoteUserBinding change to every
// user-specific ClusterWideRemoteSyncer that draws its identities from the RUB's
// namespace, so the user-specific policy can (re)create per-user RemoteTargets.
func (r *ClusterWideRemoteSyncerReconciler) findSyncersForRUB(ctx context.Context, obj client.Object) []reconcile.Request {
	rub, ok := obj.(*syngit.RemoteUserBinding)
	if !ok {
		return nil
	}

	// Only care about managed RUBs
	if rub.Labels[syngit.ManagedByLabelKey] != syngit.ManagedByLabelValue {
		return nil
	}

	cwrsList := &syngit.ClusterWideRemoteSyncerList{}
	if err := r.List(ctx, cwrsList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, cwrs := range cwrsList.Items {
		if cwrs.Spec.IdentityStoreNamespace != rub.Namespace {
			continue
		}
		if cwrs.Annotations[syngit.RtAnnotationKeyUserSpecific] != "" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cwrs.Name},
			})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterWideRemoteSyncerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorder("clusterwideremotesyncer-controller")

	r.loadFromEnv(r.Client)
	r.Namespace = r.managerNamespace

	if r.WebhookServer == nil {
		return fmt.Errorf("the ClusterWideRemoteSyncer reconciler requires a WebhookServer")
	}

	// Run inline for the same reason as the RemoteSyncer controller: a separate
	// controller watching the same object would race this one.
	r.branchTargetPolicy = &policy.BranchTargetPolicy{Client: r.Client}
	r.userSpecificPolicy = &policy.UserSpecificPolicy{Client: r.Client}

	return ctrl.NewControllerManagedBy(mgr).
		For(&syngit.ClusterWideRemoteSyncer{}).
		Watches(
			&admissionv1.ValidatingWebhookConfiguration{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForDynamicWebhook),
			builder.WithPredicates(webhookNamePredicate(r.dynamicWebhookName)),
		).
		Watches(
			&syngit.RemoteUserBinding{},
			handler.EnqueueRequestsFromMapFunc(r.findSyncersForRUB),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
