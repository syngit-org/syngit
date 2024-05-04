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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
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
	rIName := resourcesInterceptor.Name

	var prefixMsg = "[" + rINamespace + "/" + rIName + "]" + tabString

	log.Log.Info(prefixMsg + "Reconciling request received")

	// Define the webhook path
	webhookPath := "/kgio/validate/" + rINamespace + "/" + rIName
	// When ri reconciled, then create a path handled by the dynamic webhook server
	r.webhookServer.CreatePathHandler(resourcesInterceptor, webhookPath)

	// The service is located in the manager/controller namespace
	serviceName := "resources-interceptor-webhook"
	operatorNamespace := r.Namespace

	caCert, err := r.loadCACertificateFromSecret(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	var sideEffectsNone = admissionv1.SideEffectClassNone

	// Create a new ValidatingWebhook object
	webhook := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: v1.ObjectMeta{
			Name: rIName,
			// Namespace: rINamespace,
		},
		Webhooks: []admissionv1.ValidatingWebhook{
			{
				Name:                    rIName + ".kgio.com",
				AdmissionReviewVersions: []string{"v1"},
				SideEffects:             &sideEffectsNone,
				Rules:                   nsrListToRuleList(kgiov1.NSKstoNSRs(resourcesInterceptor.Spec.IncludedKinds)),
				ClientConfig: admissionv1.WebhookClientConfig{
					Service: &admissionv1.ServiceReference{
						Name:      serviceName,
						Namespace: operatorNamespace,
						Path:      &webhookPath,
					},
					CABundle: caCert,
				},
				NamespaceSelector: &v1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": rINamespace},
				},
			},
		},
	}

	// Set controller reference to own the object
	// if err := ctrl.SetControllerReference(&resourcesInterceptor, webhook, r.Scheme); err != nil {
	// 	return ctrl.Result{}, err
	// }

	// Check if the webhook already exists
	found := &admissionv1.ValidatingWebhookConfiguration{}
	err = r.Get(ctx, req.NamespacedName, found)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, err
	}

	fmt.Println(err)

	if err == nil {
		// Update the existing webhook
		found.Webhooks = webhook.Webhooks
		err = r.Update(ctx, found)
		fmt.Println(err)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		// Create a new webhook if not found
		err = r.Create(ctx, webhook)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *ResourcesInterceptorReconciler) loadCACertificateFromSecret(ctx context.Context) ([]byte, error) {
	var secret corev1.Secret
	err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: "resources-interceptor-tls"}, &secret)
	if err != nil {
		return nil, err
	}
	caCert, ok := secret.Data["tls.crt"]
	if !ok {
		return nil, fmt.Errorf("CA certificate key not found in secret")
	}
	return caCert, nil
}

func nsrListToRuleList(nsrList []kgiov1.NamespaceScopedResources) []admissionv1.RuleWithOperations {
	var scope admissionv1.ScopeType = admissionv1.NamespacedScope
	rules := []admissionv1.RuleWithOperations{}

	for _, nsr := range nsrList {
		rules = append(rules, admissionv1.RuleWithOperations{
			Operations: []admissionv1.OperationType{"CREATE", "UPDATE", "DELETE"},
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

	managerNamespace := os.Getenv("MANAGER_NAMESPACE")
	r.Namespace = managerNamespace

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &kgiov1.ResourcesInterceptor{}, includedResourcesField, func(rawObj client.Object) []string {

		resourcesInterceptor := rawObj.(*kgiov1.ResourcesInterceptor)
		gvrnsRefName := []string{}

		gvrns := kgiov1.ParsegvrnList(kgiov1.NSKstoNSRs(resourcesInterceptor.Spec.IncludedKinds))
		if len(gvrns) == 0 {
			return nil
		}

		for _, gvrn := range gvrns {
			gvrnsRefName = append(gvrnsRefName, gvrn.GroupVersionResource.Group+"/"+gvrn.GroupVersionResource.Version+"/"+gvrn.GroupVersionResource.Resource+"/"+gvrn.Name)
		}

		return gvrnsRefName
	}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &kgiov1.ResourcesInterceptor{}, nameField, func(rawObj client.Object) []string {

		resourcesInterceptor := rawObj.(*kgiov1.ResourcesInterceptor)

		return []string{resourcesInterceptor.Name}
	}); err != nil {
		return err
	}
	if err := r.createOrUpdateTLSSecret(context.Background(), mgr.GetClient()); err != nil {
		return err
	}

	// Initialize the webhookServer
	r.webhookServer = WebhookInterceptsAll{}
	r.webhookServer.Start()

	return ctrl.NewControllerManagedBy(mgr).
		For(&kgiov1.ResourcesInterceptor{}).
		// Watches(
		// 	&kgiov1.ResourcesInterceptor{},
		// 	handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {

		// 		attachedResourcesInterceptor := &kgiov1.ResourcesInterceptorList{}
		// 		listOps := &client.ListOptions{
		// 			FieldSelector: fields.OneTermEqualSelector(nameField, obj.GetName()),
		// 			Namespace:     obj.GetNamespace(),
		// 		}
		// 		err := r.List(ctx, attachedResourcesInterceptor, listOps)
		// 		if err != nil {
		// 			return []reconcile.Request{}
		// 		}

		// 		for _, resourcesInterceptor := range attachedResourcesInterceptor.Items {
		// 			ri := obj.(*kgiov1.ResourcesInterceptor)
		// 			dynamicResource := &unstructured.Unstructured{}

		// 			// Filter ONLY the resources we want to watch for -> defined in the spec of the ResourcesInterceptor
		// 			for _, gvkn := range kgiov1.ParsegvknList(resourcesInterceptor.Spec.IncludedKinds) {
		// 				fmt.Println("-----------------")
		// 				fmt.Println(*gvkn.GroupVersionKind)
		// 				dynamicResource.SetGroupVersionKind(*gvkn.GroupVersionKind)

		// 				err := ctrl.NewControllerManagedBy(mgr).
		// 					For(ri).
		// 					Watches(
		// 						dynamicResource,
		// 						handler.EnqueueRequestsFromMapFunc(r.dynamicObjectFinder),
		// 						builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		// 					).
		// 					WithLogConstructor(func(request *reconcile.Request) logr.Logger {
		// 						return mgr.GetLogger()
		// 					}).
		// 					Complete(r)
		// 				if err != nil {
		// 					mgr.GetLogger().Error(err, fmt.Sprintf("Unable to create a dynamic controller for %s", gvkn))
		// 				}
		// 			}
		// 		}

		// 		return []reconcile.Request{}
		// 	}),
		// ).
		Complete(r)
}

func (r *ResourcesInterceptorReconciler) createOrUpdateTLSSecret(ctx context.Context, c client.Client) error {
	// Generate private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %v", err)
	}

	// TODO manage with cert-manager
	// Create a certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"kgio"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0), // Valid for 1 year
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"resources-interceptor-webhook-service." + r.Namespace + ".svc"},
	}

	// Create a self-signed certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %v", err)
	}

	// Encode private key and certificate
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	// Create or update secret
	secret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "resources-interceptor-tls",
			Namespace: r.Namespace,
		},
		Data: map[string][]byte{
			"tls.key": keyPEM,
			"tls.crt": certPEM,
		},
	}

	err = c.Update(ctx, secret)
	if err != nil {
		// If the update fails, try to create the webhook
		if err := r.Create(ctx, secret); err != nil {
			return fmt.Errorf("failed to create or update secret: %v", err)
		}
	}

	return nil
}
