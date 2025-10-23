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
	"os"
	"slices"

	interceptor "github.com/syngit-org/syngit/internal/interceptor"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	"github.com/syngit-org/syngit/pkg/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	WebhookServiceName = "syngit-webhook-service"
	certificateName    = "syngit-webhook-cert"
)

// RemoteSyncerReconciler reconciles a RemoteSyncer object
type RemoteSyncerReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	webhookServer      interceptor.WebhookInterceptsAll
	dynamicWebhookName string
	Namespace          string
	devMode            bool
	devWebhookHost     string
	devWebhookCert     string
	devWebhookPort     string
	Recorder           record.EventRecorder
}

// +kubebuilder:rbac:groups=syngit.io,resources=remotesyncers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syngit.io,resources=remotesyncers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=syngit.io,resources=remotesyncers/finalizers,verbs=update
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=create;get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch;list;watch
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

func (r *RemoteSyncerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	isDeleted := false

	var rSNamespace string
	var rSName string

	// Get the RemoteSyncer Object
	var remoteSyncer syngit.RemoteSyncer
	if err := r.Get(ctx, req.NamespacedName, &remoteSyncer); err != nil {
		// does not exists -> deleted
		r.webhookServer.Unregister(req.NamespacedName)
		isDeleted = true
		rSName = req.Name
		rSNamespace = req.Namespace
		// return ctrl.Result{}, client.IgnoreNotFound(err)
	} else {
		rSNamespace = remoteSyncer.Namespace
		rSName = remoteSyncer.Name
	}

	log.Log.Info("Reconcile request",
		"resource", "remotesyncer",
		"namespace", rSNamespace,
		"name", rSName,
	)

	// Define the webhook path
	webhookPath := "/syngit/validate/" + rSNamespace + "/" + rSName
	// When rs reconciled, then create a path handled by the dynamic webhook server
	r.webhookServer.Register(remoteSyncer, webhookPath)

	// Read the content of the certificate file
	var caCert []byte
	var certError error
	if !r.devMode {
		const certPath = "/tmp/k8s-webhook-server/serving-certs/tls.crt"
		if caCert, certError = os.ReadFile(certPath); certError != nil {
			log.Log.Error(certError, fmt.Sprintf("failed to read the cert file %s", certPath))
			r.Recorder.Event(&remoteSyncer, "Warning", "WebhookCertFail", "Operator internal error : the certificate file failed to be read")
			return reconcile.Result{}, certError
		}
	} else {
		if caCert, certError = os.ReadFile(r.devWebhookCert); certError != nil {
			log.Log.Error(certError, fmt.Sprintf("failed to read the cert file %s", r.devWebhookCert))
			r.Recorder.Event(&remoteSyncer, "Warning", "WebhookCertFail", "Operator internal error : the certificate file failed to be read")
			return reconcile.Result{}, certError
		}
	}

	// The service is located in the manager/controller namespace
	operatorNamespace := r.Namespace
	clientConfig := admissionv1.WebhookClientConfig{
		Service: &admissionv1.ServiceReference{
			Name:      WebhookServiceName,
			Namespace: operatorNamespace,
			Path:      &webhookPath,
		},
		CABundle: caCert,
	}

	annotations := make(map[string]string)
	if r.devMode {
		url := "https://" + r.devWebhookHost + ":" + r.devWebhookPort + webhookPath
		clientConfig = admissionv1.WebhookClientConfig{
			URL:      &url,
			CABundle: caCert,
		}
	} else {
		annotations["cert-manager.io/inject-ca-from"] = fmt.Sprintf("%s:%s", r.Namespace, certificateName)
	}

	// Create the webhook specs for this specific RI
	webhookObjectName := r.dynamicWebhookName
	var sideEffectsNone = admissionv1.SideEffectClassNone
	webhookSpecificName := rSName + "." + rSNamespace + ".syngit.io"

	// Create a new ValidatingWebhook object
	webhook := &admissionv1.ValidatingWebhook{
		Name:                    webhookSpecificName,
		AdmissionReviewVersions: []string{"v1"},
		SideEffects:             &sideEffectsNone,
		Rules:                   remoteSyncer.Spec.ScopedResources.Rules,
		ClientConfig:            clientConfig,
		NamespaceSelector: &v1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": rSNamespace},
		},
	}
	webhookConf := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: v1.ObjectMeta{
			Name:        webhookObjectName,
			Annotations: annotations,
		},
		Webhooks: []admissionv1.ValidatingWebhook{*webhook},
	}

	webhookNamespacedName := &types.NamespacedName{
		Name: webhookObjectName,
	}

	// Check if the webhook already exists
	found := &admissionv1.ValidatingWebhookConfiguration{}
	err := r.Get(ctx, *webhookNamespacedName, found)

	condition := &v1.Condition{
		LastTransitionTime: v1.Now(),
		Type:               "WebhookReconciled",
		Status:             v1.ConditionFalse,
	}

	if err == nil {
		isExactlyTheSame := false

		// Search for the webhook spec associated to this RSy
		var currentWebhookCopy []admissionv1.ValidatingWebhook
		for _, rsyWebhook := range found.Webhooks {
			if rsyWebhook.Name != webhookSpecificName {
				currentWebhookCopy = append(currentWebhookCopy, rsyWebhook)
			} else {
				isExactlyTheSame = slices.EqualFunc(rsyWebhook.Rules, webhook.Rules, rulesAreEqual)
			}
		}
		if !isDeleted {
			currentWebhookCopy = append(currentWebhookCopy, *webhook)
		}

		// The webhook already exists and is exactly the same -> do not update
		if isExactlyTheSame {
			return reconcile.Result{}, err
		}

		// If not found, then just add the new webhook spec for this RSy
		found.Webhooks = currentWebhookCopy

		err = r.Update(ctx, found)
		if err != nil {
			r.Recorder.Event(&remoteSyncer, "Warning", "WebhookNotUpdated", "The webhook exists but has not been updated")

			condition.Reason = "WebhookNotUpdated"
			condition.Message = "The webhook exists but has not been updated"
			_ = r.updateStatus(ctx, &remoteSyncer, *condition)

			return reconcile.Result{}, err
		}
	} else {
		// Create a new webhook if not found -> if it is the first RSy to be created
		err := r.Create(ctx, webhookConf)
		if err != nil {
			r.Recorder.Event(&remoteSyncer, "Warning", "WebhookNotCreated", "The webhook does not exists and has not been created")

			condition.Reason = "WebhookNotCreated"
			condition.Message = "The webhook does not exists and has not been created"
			_ = r.updateStatus(ctx, &remoteSyncer, *condition)

			return reconcile.Result{}, err
		}
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

func (r *RemoteSyncerReconciler) webhookNamePredicate(name string) predicate.Predicate {
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

	recorder := mgr.GetEventRecorderFor("remotesyncer-controller")
	r.Recorder = recorder

	r.devMode = os.Getenv("DEV_MODE") == "true"
	r.devWebhookHost = os.Getenv("DEV_WEBHOOK_HOST")
	r.devWebhookPort = os.Getenv("DEV_WEBHOOK_PORT")
	r.devWebhookCert = os.Getenv("DEV_WEBHOOK_CERT")
	r.Namespace = os.Getenv("MANAGER_NAMESPACE")
	r.dynamicWebhookName = os.Getenv("DYNAMIC_WEBHOOK_NAME")

	// Initialize the webhookServer
	r.webhookServer = interceptor.WebhookInterceptsAll{
		K8sClient: mgr.GetClient(),
		Manager:   mgr,
	}
	r.webhookServer.Start()

	return ctrl.NewControllerManagedBy(mgr).
		For(&syngit.RemoteSyncer{}).
		Watches(
			&admissionv1.ValidatingWebhookConfiguration{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForDynamicWebhook),
			builder.WithPredicates(r.webhookNamePredicate(r.dynamicWebhookName)),
		).
		Complete(r)
}
