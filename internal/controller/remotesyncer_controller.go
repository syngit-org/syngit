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
	admissionv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// RemoteSyncerReconciler reconciles a RemoteSyncer object
type RemoteSyncerReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	webhookServer WebhookInterceptsAll
	Namespace     string
	Dev           bool
	Recorder      record.EventRecorder
}

//+kubebuilder:rbac:groups=syngit.io,resources=remotesyncers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=syngit.io,resources=remotesyncers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=syngit.io,resources=remotesyncers/finalizers,verbs=update
//+kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch;list;watch
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

func (r *RemoteSyncerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	isDeleted := false

	var rSNamespace string
	var rSName string

	// Get the RemoteSyncer Object
	var remoteSyncer syngit.RemoteSyncer
	if err := r.Get(ctx, req.NamespacedName, &remoteSyncer); err != nil {
		// does not exists -> deleted
		r.webhookServer.DestroyPathHandler(req.NamespacedName)
		isDeleted = true
		rSName = req.Name
		rSNamespace = req.Namespace
		// return ctrl.Result{}, client.IgnoreNotFound(err)
	} else {
		rSNamespace = remoteSyncer.Namespace
		rSName = remoteSyncer.Name
	}

	var prefixMsg = "[" + rSNamespace + "/" + rSName + "]"
	log.Log.Info(prefixMsg + " Reconciling request received")

	// Define the webhook path
	webhookPath := "/syngit/validate/" + rSNamespace + "/" + rSName
	// When rs reconciled, then create a path handled by the dynamic webhook server
	r.webhookServer.CreatePathHandler(remoteSyncer, webhookPath)

	// Read the content of the certificate file
	caCert, err := os.ReadFile("/tmp/k8s-webhook-server/serving-certs/tls.crt")
	if err != nil {
		log.Log.Error(err, "failed to read the cert file /tmp/k8s-webhook-server/serving-certs/tls.crt")
		r.Recorder.Event(&remoteSyncer, "Warning", "WebhookCertFail", "Operator internal error : the certificate file failed to be read")
		return reconcile.Result{}, err
	}

	// The service is located in the manager/controller namespace
	serviceName := "syngit-remote-syncer-webhook-service"
	operatorNamespace := r.Namespace
	clientConfig := admissionv1.WebhookClientConfig{
		Service: &admissionv1.ServiceReference{
			Name:      serviceName,
			Namespace: operatorNamespace,
			Path:      &webhookPath,
		},
		CABundle: caCert,
	}

	annotations := make(map[string]string)
	// Development mode
	if r.Dev {
		url := "https://172.17.0.1:9444" + webhookPath
		clientConfig = admissionv1.WebhookClientConfig{
			URL:      &url,
			CABundle: caCert,
		}
	}
	if !r.Dev {
		annotations["cert-manager.io/inject-ca-from"] = "operator-webhook-cert"
	}

	// Create the webhook specs for this specific RI
	webhookObjectName := os.Getenv("DYNAMIC_WEBHOOK_NAME")
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
		// FailurePolicy: DON'T FAIL,
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
	err = r.Get(ctx, *webhookNamespacedName, found)

	condition := &v1.Condition{
		LastTransitionTime: v1.Now(),
		Type:               "NotReady",
		Status:             "False",
	}

	if err == nil {
		// Search for the webhook spec associated to this RI
		var currentWebhookCopy []admissionv1.ValidatingWebhook
		for _, riWebhook := range found.Webhooks {
			if riWebhook.Name != webhookSpecificName {
				currentWebhookCopy = append(currentWebhookCopy, riWebhook)
			}
		}
		if !isDeleted {
			currentWebhookCopy = append(currentWebhookCopy, *webhook)
		}

		// If not found, then just add the new webhook spec for this RI
		found.Webhooks = currentWebhookCopy

		err = r.Update(ctx, found)
		if err != nil {
			r.Recorder.Event(&remoteSyncer, "Warning", "WebhookNotUpdated", "The webhook exists but has not been updated")

			condition.Reason = "WebhookNotUpdated"
			condition.Message = "The webhook exists but has not been updated"
			_ = r.updateConditions(ctx, &remoteSyncer, *condition)

			return reconcile.Result{}, err
		}
	} else {
		// Create a new webhook if not found -> if it is the first RI to be created
		err := r.Create(ctx, webhookConf)
		if err != nil {
			r.Recorder.Event(&remoteSyncer, "Warning", "WebhookNotCreated", "The webhook does not exists and has not been created")

			condition.Reason = "WebhookNotCreated"
			condition.Message = "The webhook does not exists and has not been created"
			_ = r.updateConditions(ctx, &remoteSyncer, *condition)

			return reconcile.Result{}, err
		}
	}

	condition.Type = "Ready"
	condition.Reason = "WebhookUpdated"
	condition.Message = "The resources have been successfully assigned to the webhook"
	condition.Status = "True"
	_ = r.updateConditions(ctx, &remoteSyncer, *condition)

	return ctrl.Result{}, nil
}

func (r *RemoteSyncerReconciler) updateConditions(ctx context.Context, rs *syngit.RemoteSyncer, condition v1.Condition) error {
	added := false
	var conditions []v1.Condition
	for _, cond := range rs.Status.Conditions {
		if cond.Type == condition.Type {
			if cond.Reason != condition.Reason {
				r.Recorder.Event(rs, "Normal", "WebhookUpdated", "The resources have been successfully assigned to the webhook")
				conditions = append(conditions, condition)
			} else {
				conditions = append(conditions, cond)
			}
			added = true
		} else {
			conditions = append(conditions, cond)
		}
	}
	if !added {
		conditions = append(conditions, condition)
	}

	rs.Status.Conditions = conditions
	if err := r.Status().Update(ctx, rs); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemoteSyncerReconciler) SetupWithManager(mgr ctrl.Manager) error {

	recorder := mgr.GetEventRecorderFor("remotesyncer-controller")
	r.Recorder = recorder

	managerNamespace := os.Getenv("MANAGER_NAMESPACE")
	dev := os.Getenv("DEV")
	r.Namespace = managerNamespace
	r.Dev = false
	if dev == "true" {
		r.Dev = true
	}

	// Initialize the webhookServer
	r.webhookServer = WebhookInterceptsAll{
		k8sClient: mgr.GetClient(),
		dev:       r.Dev,
	}
	r.webhookServer.Start()

	return ctrl.NewControllerManagedBy(mgr).
		For(&syngit.RemoteSyncer{}).
		Complete(r)
}
