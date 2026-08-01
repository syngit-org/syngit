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
	"errors"
	"fmt"
	"slices"

	interceptor "github.com/syngit-org/syngit/internal/interceptor"
	"github.com/syngit-org/syngit/internal/policy"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"github.com/syngit-org/syngit/pkg/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// RemoteSyncerReconciler reconciles a RemoteSyncer object. It is the single
// controller that owns RemoteSyncer: it manages the dynamic webhook
// configuration and runs the RemoteSyncer-scoped policies (branch-target and
// user-specific).
type RemoteSyncerReconciler struct {
	client.Client
	// Owns this syncer's entry in the shared dynamic webhook configuration.
	dynamicWebhookManager

	Scheme *runtime.Scheme
	// WebhookServer is the interception server shared with the
	// ClusterWideRemoteSyncer controller. Required.
	WebhookServer *interceptor.WebhookInterceptsAll
	Namespace     string
	Recorder      events.EventRecorder

	branchTargetPolicy *policy.BranchTargetPolicy
	userSpecificPolicy *policy.UserSpecificPolicy
}

// +kubebuilder:rbac:groups=syngit.io,resources=remotesyncers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syngit.io,resources=remotesyncers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=syngit.io,resources=remotesyncers/finalizers,verbs=update
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=create;get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch;list;watch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch;list;watch
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases,verbs=get;list;watch

func (r *RemoteSyncerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Core: reconcile the dynamic webhook configuration. This also handles the
	// already-deleted case (Get returns NotFound) by removing the syncer's
	// webhook entry.
	coreResult, coreErr := r.reconcileWebhook(ctx, req)

	// Policies run only while the object still exists. Once it is fully gone
	// (NotFound), its finalizers are already removed, so there is nothing to do.
	var remoteSyncer syngit.RemoteSyncer
	if err := r.Get(ctx, req.NamespacedName, &remoteSyncer); err != nil {
		return coreResult, coreErr
	}

	// Instantiated at the Syncer interface rather than at *RemoteSyncer so that
	// these same policy instances also serve the ClusterWideRemoteSyncer
	// controller.
	polResult, polErr := policy.RunPolicies[syngit.Syncer](ctx, r.Client, &remoteSyncer,
		[]policy.Policy[syngit.Syncer]{r.branchTargetPolicy, r.userSpecificPolicy})

	return utils.MergeResults(coreResult, polResult), errors.Join(coreErr, polErr)
}

// reconcileWebhook manages the dynamic ValidatingWebhookConfiguration for this
// RemoteSyncer (and removes its entry when the object no longer exists).
func (r *RemoteSyncerReconciler) reconcileWebhook(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	webhookPath := interceptor.RemoteSyncerWebhookPath(req.NamespacedName)

	// Get the RemoteSyncer Object
	isDeleted := false
	var remoteSyncer syngit.RemoteSyncer
	if err := r.Get(ctx, req.NamespacedName, &remoteSyncer); err != nil {
		// does not exist -> deleted
		r.WebhookServer.Unregister(webhookPath)
		isDeleted = true
	} else {
		// Only register a syncer that still exists: registering here on the
		// deleted path would put a zero-valued RemoteSyncer straight back into
		// the cache we just cleared.
		r.WebhookServer.Register(remoteSyncer, webhookPath)
	}

	log.Log.Info("Reconcile request",
		"resource", "remotesyncer",
		"namespace", req.Namespace,
		"name", req.Name,
	)

	entry := dynamicWebhookEntry{
		name:  req.Name + "." + req.Namespace + ".syngit.io",
		path:  webhookPath,
		rules: remoteSyncer.Spec.ScopedResources.Rules,
		// A namespaced RemoteSyncer only ever intercepts its own namespace.
		namespaceSelector: &v1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": req.Namespace},
		},
		objectSelector: remoteSyncer.Spec.ScopedResources.ObjectSelector,
	}

	condition := &v1.Condition{
		LastTransitionTime: v1.Now(),
		Type:               "WebhookReconciled",
		Status:             v1.ConditionFalse,
	}

	if err := r.upsert(ctx, entry, isDeleted); err != nil {
		r.Recorder.Eventf(&remoteSyncer, nil, "Warning", "WebhookNotUpdated", "The dynamic webhook has not been updated", "")

		condition.Reason = "WebhookNotUpdated"
		condition.Message = "The dynamic webhook has not been updated: " + err.Error()
		_ = r.updateStatus(ctx, &remoteSyncer, *condition)

		return reconcile.Result{}, err
	}

	if isDeleted {
		return ctrl.Result{}, nil
	}

	condition.Reason = "WebhookUpdated"
	condition.Message = "The resources have been successfully assigned to the webhook"
	condition.Status = v1.ConditionTrue
	_ = r.updateStatus(ctx, &remoteSyncer, *condition)

	return ctrl.Result{}, nil
}

func rulesAreEqual(r1, r2 admissionv1.RuleWithOperations) bool {
	if !slices.Equal(r1.APIGroups, r2.APIGroups) {
		return false
	}
	if !slices.Equal(r1.APIVersions, r2.APIVersions) {
		return false
	}
	if !slices.Equal(r1.Resources, r2.Resources) {
		return false
	}
	if !slices.Equal(r1.Operations, r2.Operations) {
		return false
	}
	return true
}

func (r *RemoteSyncerReconciler) updateStatus(ctx context.Context, remoteSyncer *syngit.RemoteSyncer, condition v1.Condition) error {
	conditions := utils.TypeBasedConditionUpdater(remoteSyncer.Status.DeepCopy().Conditions, condition)

	remoteSyncer.Status.Conditions = conditions
	if err := r.Status().Update(ctx, remoteSyncer); err != nil {
		return err
	}
	return nil
}

func (r *RemoteSyncerReconciler) findObjectsForDynamicWebhook(ctx context.Context, webhook client.Object) []reconcile.Request {
	attachedRemoteSyncers := &syngit.RemoteSyncerList{}
	listOps := &client.ListOptions{
		Namespace: "",
	}
	// List all the RemoteSyncers of the cluster
	err := r.List(ctx, attachedRemoteSyncers, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	// Returns back all the RemoteSyncer of the cluster
	requests := make([]reconcile.Request, len(attachedRemoteSyncers.Items))
	for i, item := range attachedRemoteSyncers.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}

// findRemoteSyncersForRUB maps a managed RemoteUserBinding change to reconcile
// requests for every user-specific RemoteSyncer in its namespace, so the
// user-specific policy can (re)create per-user RemoteTargets.
func (r *RemoteSyncerReconciler) findRemoteSyncersForRUB(ctx context.Context, obj client.Object) []reconcile.Request {
	rub, ok := obj.(*syngit.RemoteUserBinding)
	if !ok {
		return nil
	}

	// Only care about managed RUBs
	if rub.Labels[syngit.ManagedByLabelKey] != syngit.ManagedByLabelValue {
		return nil
	}

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

// Narrows a ValidatingWebhookConfiguration watch to the one
// shared dynamic configuration.
func webhookNamePredicate(name string) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetName() == name
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectNew.GetName() == name
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetName() == name
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemoteSyncerReconciler) SetupWithManager(mgr ctrl.Manager) error {

	recorder := mgr.GetEventRecorder("remotesyncer-controller")
	r.Recorder = recorder

	r.loadFromEnv(r.Client)
	r.Namespace = r.managerNamespace

	if r.WebhookServer == nil {
		return fmt.Errorf("the RemoteSyncer reconciler requires a WebhookServer")
	}

	// The branch-target and user-specific policies are run inline by this
	// controller instead of being separate controllers that also watch
	// RemoteSyncer (which would race us).
	r.branchTargetPolicy = &policy.BranchTargetPolicy{Client: r.Client}
	r.userSpecificPolicy = &policy.UserSpecificPolicy{Client: r.Client}

	return ctrl.NewControllerManagedBy(mgr).
		For(&syngit.RemoteSyncer{}).
		Watches(
			&admissionv1.ValidatingWebhookConfiguration{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForDynamicWebhook),
			builder.WithPredicates(webhookNamePredicate(r.dynamicWebhookName)),
		).
		Watches(
			&syngit.RemoteUserBinding{},
			handler.EnqueueRequestsFromMapFunc(r.findRemoteSyncersForRUB),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
