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
	"slices"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kgiov1 "dams.kgio/kgio/api/v1"
)

// ResourcesInterceptorReconciler reconciles a ResourcesInterceptor object
type ResourcesInterceptorReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	webhookServer WebhookInterceptsAll
	Namespace     string
	Dev           bool
}

//+kubebuilder:rbac:groups=kgio.dams.kgio,resources=resourcesinterceptors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kgio.dams.kgio,resources=resourcesinterceptors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kgio.dams.kgio,resources=resourcesinterceptors/finalizers,verbs=update
//+kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=create;get;list;watch;update;patch;delete

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
	rIName := resourcesInterceptor.Name

	var prefixMsg = "[" + rINamespace + "/" + rIName + "]" + tabString

	log.Log.Info(prefixMsg + "Reconciling request received")

	// Define the webhook path
	webhookPath := "/kgio/validate/" + rINamespace + "/" + rIName
	// When ri reconciled, then create a path handled by the dynamic webhook server
	r.webhookServer.CreatePathHandler(resourcesInterceptor, webhookPath)

	// Read the content of the certificate file
	caCert, err := os.ReadFile("/tmp/k8s-webhook-server/serving-certs/tls.crt")
	if err != nil {
		log.Log.Error(err, "failed to read the cert file /tmp/k8s-webhook-server/serving-certs/tls.crt")
		panic(err) // handle error if unable to read file
	}

	// The service is located in the manager/controller namespace
	serviceName := "webhook-pusher-service"
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
	webhookObjectName := "resourcesinterceptor.kgio.com"
	var sideEffectsNone = admissionv1.SideEffectClassNone
	webhookSpecificName := rIName + ".kgio.com"

	// Create a new ValidatingWebhook object
	webhook := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: v1.ObjectMeta{
			Name: webhookObjectName,
			Annotations: annotations,
		},
		Webhooks: []admissionv1.ValidatingWebhook{
			{
				Name:                    webhookSpecificName,
				AdmissionReviewVersions: []string{"v1"},
				SideEffects:             &sideEffectsNone,
				Rules:                   nsrListToRuleList(kgiov1.NSRPstoNSRs(resourcesInterceptor.Spec.IncludedResources), resourcesInterceptor.Spec.DeepCopy().Operations),
				ClientConfig:            clientConfig,
				NamespaceSelector: &v1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": rINamespace},
				},
				// FailurePolicy: DON'T FAIL,
			},
		},
	}

	// Set controller reference to own the object
	// if err := ctrl.SetControllerReference(&resourcesInterceptor, webhook, r.Scheme); err != nil {
	// 	return ctrl.Result{}, err
	// }

	webhookNamespacedName := &types.NamespacedName{
		Name: webhookObjectName,
	}

	// Check if the webhook already exists
	found := &admissionv1.ValidatingWebhookConfiguration{}
	err = r.Get(ctx, *webhookNamespacedName, found)
	// if err != nil && client.IgnoreNotFound(err) != nil {
	// 	return reconcile.Result{}, err
	// }

	if err == nil {
		// Search for the webhook spec associated to this RI
		foundRIWebhook := false
		var currentWebhookCopy []admissionv1.ValidatingWebhook
		for i, riWebhook := range found.Webhooks {
			if riWebhook.Name == webhookSpecificName {
				foundRIWebhook = true
				currentWebhookCopy = slices.Delete(found.Webhooks, i, 1)
				currentWebhookCopy = append(currentWebhookCopy, webhook.Webhooks[0])
			}
		}
		// If not found, then just add the new webhook spec for this RI
		if !foundRIWebhook {
			found.Webhooks = append(found.Webhooks, webhook.Webhooks[0])
		} else {
			found.Webhooks = currentWebhookCopy
		}

		err = r.Update(ctx, found)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		// Create a new webhook if not found -> if it is the first RI to be created
		err := r.Create(ctx, webhook)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func nsrListToRuleList(nsrList []kgiov1.NamespaceScopedResources, operations []admissionv1.OperationType) []admissionv1.RuleWithOperations {
	var scope admissionv1.ScopeType = admissionv1.NamespacedScope
	rules := []admissionv1.RuleWithOperations{}

	for _, nsr := range nsrList {
		rules = append(rules, admissionv1.RuleWithOperations{
			Operations: operations,
			Rule: admissionv1.Rule{
				APIGroups:   nsr.APIGroups,
				APIVersions: nsr.APIVersions,
				Resources:   nsr.Resources,
				Scope:       &scope,
			},
		})
	}

	return rules
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourcesInterceptorReconciler) SetupWithManager(mgr ctrl.Manager) error {

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
		dev: r.Dev,
	}
	r.webhookServer.Start()

	return ctrl.NewControllerManagedBy(mgr).
		For(&kgiov1.ResourcesInterceptor{}).
		Complete(r)
}
