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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
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

	kgiov1 "dams.kgio/kgio/api/v1"
)

// GitRemoteReconciler reconciles a GitRemote object
type GitRemoteReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Namespace string
}

func (r *GitRemoteReconciler) updateStatus(ctx context.Context, gitRemote *kgiov1.GitRemote) error {
	if err := r.Status().Update(ctx, gitRemote); err != nil {
		return err
	}
	return nil
}

func (r *GitRemoteReconciler) setServerConfiguration(ctx context.Context, gitRemote *kgiov1.GitRemote) (kgiov1.GitServerConfiguration, error) {

	gpc := &kgiov1.GitServerConfiguration{
		Inherited:              false,
		AuthenticationEndpoint: "",
		CaBundle:               "",
		InsecureSkipTlsVerify:  false,
	}

	// STEP 1 : Check the config map ref
	var cm corev1.ConfigMap
	if gitRemote.Spec.CustomGitServerConfigRef.Name != "" {
		// It is defined in the GitRemote object
		namespacedName := types.NamespacedName{Namespace: gitRemote.Namespace, Name: gitRemote.Spec.CustomGitServerConfigRef.Name}
		if err := r.Get(ctx, namespacedName, &cm); err != nil {
			gitRemote.Status.ConnexionStatus.Status = kgiov1.GitConfigNotFound
			gitRemote.Status.ConnexionStatus.Details = "ConfigMap name: " + gitRemote.Spec.CustomGitServerConfigRef.Name
			return *gpc, err
		}
	} else {
		// It is not defined in the GitRemote object -> look for the default configmap of the operator
		namespacedName := types.NamespacedName{Namespace: r.Namespace, Name: gitRemote.Spec.GitBaseDomainFQDN}
		if err := r.Get(ctx, namespacedName, &cm); err != nil {
			gitRemote.Status.ConnexionStatus.Status = kgiov1.GitConfigNotFound
			gitRemote.Status.ConnexionStatus.Details = "Configuration reference not found in the current GitRemote; ConfigMap " + gitRemote.Spec.GitBaseDomainFQDN + " in the namespace of the operator not found as well"
			return *gpc, err
		}
		gpc.Inherited = true
	}

	// STEP 2 : Build the GitServerConfiguration

	// Parse the ConfigMap
	serverConf, err := parseConfigMap(cm)
	if err != nil {
		gitRemote.Status.ConnexionStatus.Status = kgiov1.GitConfigParseError
		gitRemote.Status.ConnexionStatus.Details = err.Error()
		return *gpc, err
	}

	if gitRemote.Spec.InsecureSkipTlsVerify != serverConf.InsecureSkipTlsVerify {
		serverConf.InsecureSkipTlsVerify = gitRemote.Spec.InsecureSkipTlsVerify
	}

	*gpc = serverConf

	return *gpc, nil
}

// +kubebuilder:rbac:groups=kgio.dams.kgio,resources=gitremotes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kgio.dams.kgio,resources=gitremotes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kgio.dams.kgio,resources=gitremotes/finalizers,verbs=update
// +kubebuilder:rbac:groups=corev1,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=corev1,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
func (r *GitRemoteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// Get the GitRemote Object
	var gitRemote kgiov1.GitRemote
	if err := r.Get(ctx, req.NamespacedName, &gitRemote); err != nil {
		// does not exists -> deleted
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	gRNamespace := gitRemote.Namespace
	gRName := gitRemote.Name

	var prefixMsg = "[" + gRNamespace + "/" + gRName + "]"
	log.Log.Info(prefixMsg + " Reconciling request received")

	// Get the referenced Secret
	var secret corev1.Secret
	namespacedNameSecret := types.NamespacedName{Namespace: req.Namespace, Name: gitRemote.Spec.SecretRef.Name}
	if err := r.Get(ctx, namespacedNameSecret, &secret); err != nil {
		gitRemote.Status.SecretBoundStatus = kgiov1.SecretNotFound
		gitRemote.Status.ConnexionStatus.Status = ""
		r.updateStatus(ctx, &gitRemote)
		return ctrl.Result{}, err
	}
	gitRemote.Status.SecretBoundStatus = kgiov1.SecretBound
	username := string(secret.Data["username"])

	// Update configuration
	gpc, err := r.setServerConfiguration(ctx, &gitRemote)
	if err != nil {
		errUpdate := r.updateStatus(ctx, &gitRemote)
		return ctrl.Result{}, errUpdate
	}
	gitRemote.Status.GitServerConfiguration = gpc
	errUpdate := r.updateStatus(ctx, &gitRemote)
	if errUpdate != nil {
		return ctrl.Result{}, errUpdate
	}

	if gitRemote.Spec.TestAuthentication {

		// Check if the referenced Secret is a basic-auth type
		if secret.Type != corev1.SecretTypeBasicAuth {
			err := errors.New("secret type is not BasicAuth")
			gitRemote.Status.SecretBoundStatus = kgiov1.SecretWrongType
			return ctrl.Result{}, err
		}

		// Get the username and password from the Secret
		gitRemote.Status.GitUser = username
		PAToken := string(secret.Data["password"])

		// If test auth -> the endpoint must exists
		authenticationEndpoint := gpc.AuthenticationEndpoint
		if authenticationEndpoint == "" {
			errMsg := ""
			if gpc.Inherited {
				errMsg = "git provider not found in the " + gitRemote.Spec.GitBaseDomainFQDN + " ConfigMap in the namespace of the operator"
			} else {
				errMsg = "git provider not found in the " + gitRemote.Spec.CustomGitServerConfigRef.Name + " ConfigMap"
			}
			gitRemote.Status.ConnexionStatus.Status = kgiov1.GitUnsupported
			gitRemote.Status.ConnexionStatus.Details = errMsg
			errUpdate := r.updateStatus(ctx, &gitRemote)
			return ctrl.Result{}, errUpdate
		}

		// Perform Git provider authentication check
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM([]byte(gpc.CaBundle)); !ok {
			gitRemote.Status.ConnexionStatus.Status = kgiov1.GitConfigParseError
			gitRemote.Status.ConnexionStatus.Details = "the certificate should be base64-encoded (in PEM format)"
			errUpdate := r.updateStatus(ctx, &gitRemote)
			return ctrl.Result{}, errUpdate
		}
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:            caCertPool,
				InsecureSkipVerify: gpc.InsecureSkipTlsVerify,
			},
		}
		httpClient := &http.Client{
			Transport: transport,
		}
		gitReq, err := http.NewRequest("GET", authenticationEndpoint, nil)
		if err != nil {
			gitRemote.Status.ConnexionStatus.Status = kgiov1.GitServerError
			gitRemote.Status.ConnexionStatus.Details = "Internal operator error : cannot create the http request " + err.Error()
			errUpdate := r.updateStatus(ctx, &gitRemote)
			return ctrl.Result{}, errUpdate
		}
		gitReq.Header.Add("Private-Token", PAToken)

		resp, err := httpClient.Do(gitReq)
		if err != nil {
			gitRemote.Status.ConnexionStatus.Status = kgiov1.GitServerError
			gitRemote.Status.ConnexionStatus.Details = "Internal operator error : the request cannot be processed " + err.Error()
			errUpdate := r.updateStatus(ctx, &gitRemote)
			return ctrl.Result{}, errUpdate
		}
		defer resp.Body.Close()

		gitRemote.Status.ConnexionStatus.Details = ""

		// Check the response status code
		if resp.StatusCode == http.StatusOK {
			// Authentication successful
			gitRemote.Status.ConnexionStatus.Status = kgiov1.GitConnected
			gitRemote.Status.LastAuthTime = metav1.Now()
			r.Recorder.Event(&gitRemote, "Normal", "Connected", "Auth succeeded")
		} else if resp.StatusCode == http.StatusUnauthorized {
			// Unauthorized: bad credentials
			gitRemote.Status.ConnexionStatus.Status = kgiov1.GitUnauthorized
			r.Recorder.Event(&gitRemote, "Warning", "AuthFailed", "Auth failed - unauthorized")
		} else if resp.StatusCode == http.StatusForbidden {
			// Forbidden : Not enough permission
			gitRemote.Status.ConnexionStatus.Status = kgiov1.GitForbidden
			r.Recorder.Event(&gitRemote, "Warning", "AuthFailed", "Auth failed - forbidden")
		} else if resp.StatusCode == http.StatusInternalServerError {
			// Server error: a server error happened
			gitRemote.Status.ConnexionStatus.Status = kgiov1.GitServerError
			r.Recorder.Event(&gitRemote, "Warning", "AuthFailed", "Auth failed - server error")
		} else {
			// Handle other status codes if needed
			gitRemote.Status.ConnexionStatus.Status = kgiov1.GitUnexpectedStatus
			r.Recorder.Event(&gitRemote, "Warning", "AuthFailed",
				fmt.Sprintf("Auth failed - unexpected response - %s", resp.Status))
		}
	}

	// Update the status of GitRemote
	r.updateStatus(ctx, &gitRemote)

	return ctrl.Result{}, nil
}

func parseConfigMap(configMap corev1.ConfigMap) (kgiov1.GitServerConfiguration, error) {
	gitServerConf := &kgiov1.GitServerConfiguration{}
	for key, value := range configMap.Data {
		switch key {
		case "authenticationEndpoint":
			gitServerConf.AuthenticationEndpoint = value
		case "caBundle":
			gitServerConf.CaBundle = value
		case "insecureSkipTlsVerify":
			if value == "true" {
				gitServerConf.InsecureSkipTlsVerify = true
			} else {
				gitServerConf.InsecureSkipTlsVerify = false
			}
		default:
			return *gitServerConf, errors.New("wrong key " + key + " found in the git server configmap " + configMap.Namespace + "/" + configMap.Name)
		}
	}

	return *gitServerConf, nil
}

func (r *GitRemoteReconciler) findObjectsForSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	attachedGitRemotes := &kgiov1.GitRemoteList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(secretRefField, secret.GetName()),
		Namespace:     secret.GetNamespace(),
	}
	err := r.List(ctx, attachedGitRemotes, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(attachedGitRemotes.Items))
	for i, item := range attachedGitRemotes.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}

func (r *GitRemoteReconciler) findObjectsForGitProviderConfig(ctx context.Context, configMap client.Object) []reconcile.Request {
	attachedGitRemotes := &kgiov1.GitRemoteList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(gitProviderConfigRefField, configMap.GetName()),
		Namespace:     configMap.GetNamespace(),
	}
	err := r.List(ctx, attachedGitRemotes, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(attachedGitRemotes.Items))
	for i, item := range attachedGitRemotes.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}

func (r *GitRemoteReconciler) findObjectsForRootConfigMap(ctx context.Context, configMap client.Object) []reconcile.Request {
	attachedGitRemotes := &kgiov1.GitRemoteList{}
	listOps := &client.ListOptions{}
	err := r.List(ctx, attachedGitRemotes, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(attachedGitRemotes.Items))
	for i, item := range attachedGitRemotes.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}

func (r *GitRemoteReconciler) gitEndpointsConfigCreation(e event.CreateEvent) bool {
	configMap, ok := e.Object.(*corev1.ConfigMap)
	if !ok {
		return false
	}
	return configMap.Namespace == r.Namespace && strings.Contains(configMap.Name, ".")
}

func (r *GitRemoteReconciler) gitEndpointsConfigUpdate(e event.UpdateEvent) bool {
	configMap, ok := e.ObjectNew.(*corev1.ConfigMap)
	if !ok {
		return false
	}
	return configMap.Namespace == r.Namespace && strings.Contains(configMap.Name, ".")
}

func (r *GitRemoteReconciler) gitEndpointsConfigDeletion(e event.DeleteEvent) bool {
	configMap, ok := e.Object.(*corev1.ConfigMap)
	if !ok {
		return false
	}
	return configMap.Namespace == r.Namespace && strings.Contains(configMap.Name, ".")
}

const (
	secretRefField            = ".spec.secretRef.name"
	gitProviderConfigRefField = ".spec.CustomGitServerConfigRef.name"
)

// SetupWithManager sets up the controller with the Manager.
func (r *GitRemoteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &kgiov1.GitRemote{}, secretRefField, func(rawObj client.Object) []string {
		// Extract the Secret name from the GitRemote Spec, if one is provided
		gitRemote := rawObj.(*kgiov1.GitRemote)
		if gitRemote.Spec.SecretRef.Name == "" {
			return nil
		}
		return []string{gitRemote.Spec.SecretRef.Name}
	}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &kgiov1.GitRemote{}, gitProviderConfigRefField, func(rawObj client.Object) []string {
		// Extract the ConfigMap name from the GitRemote Spec, if one is provided
		gitRemote := rawObj.(*kgiov1.GitRemote)
		if gitRemote.Spec.CustomGitServerConfigRef.Name == "" {
			return nil
		}
		return []string{gitRemote.Spec.CustomGitServerConfigRef.Name}
	}); err != nil {
		return err
	}

	// Recorder to manage events
	recorder := mgr.GetEventRecorderFor("gitremote-controller")
	r.Recorder = recorder

	managerNamespace := os.Getenv("MANAGER_NAMESPACE")
	r.Namespace = managerNamespace

	configMapPredicates := predicate.Funcs{
		CreateFunc: r.gitEndpointsConfigCreation,
		UpdateFunc: r.gitEndpointsConfigUpdate,
		DeleteFunc: r.gitEndpointsConfigDeletion,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&kgiov1.GitRemote{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSecret),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForGitProviderConfig),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForRootConfigMap),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}, configMapPredicates),
		).
		Complete(r)
}
