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
	"net/http"
	"os"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
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
	gitBaseDomainFQDN := gitRemote.Spec.GitBaseDomainFQDN
	log.Log.Info("[" + gRNamespace + "/" + gRName + "] Reconciling request received")

	// Get the referenced Secret
	var secret corev1.Secret
	retrievedSecret := types.NamespacedName{Namespace: req.Namespace, Name: gitRemote.Spec.SecretRef.Name}
	if err := r.Get(ctx, retrievedSecret, &secret); err != nil {
		log.Log.Error(nil, "["+gRNamespace+"/"+gRName+"] Secret not found with the name "+gitRemote.Spec.SecretRef.Name)
		gitRemote.Status.ConnexionStatus = kgiov1.Disconnected
		if err := r.Status().Update(ctx, &gitRemote); err != nil {
			// log.Log.Error(err, "["+gRNamespace+"/"+gRName+"] Failed to update status")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		return ctrl.Result{}, err
	}
	username := string(secret.Data["username"])
	log.Log.Info("[" + gRNamespace + "/" + gRName + "] Secret found, username : " + username)

	if gitRemote.Spec.TestAuthentication {
		log.Log.Info("[" + gRNamespace + "/" + gRName + "] Auth check on " + gitBaseDomainFQDN + " using the token associated to " + username)

		// Check if the referenced Secret is a basic-auth type
		if secret.Type != corev1.SecretTypeBasicAuth {
			err := fmt.Errorf("secret type is not BasicAuth")
			log.Log.Error(nil, "["+gRNamespace+"/"+gRName+"] The secret type is not BasicAuth, found: "+string(secret.Type))
			return ctrl.Result{}, err
		}

		// Get the username and password from the Secret
		gitRemote.Status.GitUserID = username
		PAToken := string(secret.Data["password"])

		// Fetch the ConfigMap
		// controllerNamespace := ctx.Value("controllerNamespace").(string)
		configMap := &corev1.ConfigMap{}
		configMapName := types.NamespacedName{Namespace: r.Namespace, Name: "git-providers-endpoints"}
		if err := r.Get(ctx, configMapName, configMap); err != nil {
			log.Log.Error(nil, "["+gRNamespace+"/"+gRName+"] ConfigMap not found with the name git-providers-endpoints in the operator's namespace")
			return ctrl.Result{}, err
		}

		// Parse the ConfigMap
		providers, err := parseConfigMap(*configMap)
		if err != nil {
			log.Log.Error(nil, "["+gRNamespace+"/"+gRName+"] Failed to parse ConfigMap")
			return ctrl.Result{}, err
		}

		// Determine Git provider based on GitBaseDomainFQDN
		var apiEndpoint string
		var forbiddenMessage kgiov1.GitRemoteConnexionStatus
		forbiddenMessage = kgiov1.Forbidden
		gitProvider := gitRemote.Spec.GitProvider

		for providerName, providerData := range providers {
			if gitRemote.Spec.GitProvider == providerName {
				apiEndpoint = providerData.Authentication
				forbiddenMessage = kgiov1.Forbidden
			}
		}
		if apiEndpoint == "" {
			if gitRemote.Spec.CustomGitProvider.Authentication != "" {
				apiEndpoint = gitRemote.Spec.CustomGitProvider.Authentication
			} else {
				err := fmt.Errorf("unsupported git provider")
				log.Log.Error(nil, "["+gRNamespace+"/"+gRName+"] Unsupported Git provider : "+string(gitRemote.Spec.GitProvider))
				return ctrl.Result{}, err
			}
		}

		log.Log.Info("[" + gRNamespace + "/" + gRName + "] Process authentication checking on this endpoint : " + apiEndpoint)

		// Perform Git provider authentication check
		httpClient := &http.Client{}
		gitReq, err := http.NewRequest("GET", apiEndpoint, nil)
		if err != nil {
			log.Log.Error(nil, "["+gRNamespace+"/"+gRName+"] Failed to create Git Auth Test request")
			return ctrl.Result{}, err
		}
		gitReq.Header.Add("Private-Token", PAToken)

		resp, err := httpClient.Do(gitReq)
		if err != nil {
			log.Log.Error(nil, "["+gRNamespace+"/"+gRName+"] Failed to perform the Git Auth Test request; cannot communicate with the remote Git server (%s)", gitProvider)
			return ctrl.Result{}, err
		}
		defer resp.Body.Close()

		// Check the response status code
		connexionError := fmt.Errorf("status code : %d", resp.StatusCode)
		if resp.StatusCode == http.StatusOK {
			// Authentication successful
			connexionError = nil
			gitRemote.Status.ConnexionStatus = kgiov1.Connected
			gitRemote.Status.LastAuthTime = metav1.Now()
			log.Log.Info("[" + gRNamespace + "/" + gRName + "] ✅ Auth succeeded - " + username + " connected")
			r.Recorder.Event(&gitRemote, "Normal", "Connected", "Auth succeeded")
		} else if resp.StatusCode == http.StatusUnauthorized {
			// Unauthorized: bad credentials
			gitRemote.Status.ConnexionStatus = kgiov1.Unauthorized
			log.Log.Error(connexionError, "["+gRNamespace+"/"+gRName+"] ❌ Auth failed - Unauthorized")
			r.Recorder.Event(&gitRemote, "Warning", "AuthFailed", "Auth failed - unauthorized")
		} else if resp.StatusCode == http.StatusForbidden {
			// Forbidden : Not enough permission
			gitRemote.Status.ConnexionStatus = forbiddenMessage
			log.Log.Error(connexionError, "["+gRNamespace+"/"+gRName+"] ❌ Auth failed - "+string(forbiddenMessage))
			r.Recorder.Event(&gitRemote, "Warning", "AuthFailed", "Auth failed - forbidden")
		} else if resp.StatusCode == http.StatusInternalServerError {
			// Server error: a server error happened
			gitRemote.Status.ConnexionStatus = kgiov1.ServerError
			log.Log.Error(connexionError, "["+gRNamespace+"/"+gRName+"] ❌ Auth failed - "+gitBaseDomainFQDN+" returns a Server Error")
			r.Recorder.Event(&gitRemote, "Warning", "AuthFailed", "Auth failed - server error")
		} else {
			// Handle other status codes if needed
			gitRemote.Status.ConnexionStatus = kgiov1.UnexpectedStatus
			log.Log.Error(connexionError, "["+gRNamespace+"/"+gRName+"] ❌ Auth failed - Unexpected response from "+string(gitProvider))
			r.Recorder.Event(&gitRemote, "Warning", "AuthFailed",
				fmt.Sprintf("Auth failed - unexpected response - %s", resp.Status))
		}
	}

	// Update the status of GitRemote
	if err := r.Status().Update(ctx, &gitRemote); err != nil {
		log.Log.Error(nil, "["+gRNamespace+"/"+gRName+"] Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func parseConfigMap(configMap corev1.ConfigMap) (map[string]kgiov1.GitProvider, error) {
	providers := make(map[string]kgiov1.GitProvider)

	for key, value := range configMap.Data {
		var gitProvider kgiov1.GitProvider

		if err := yaml.Unmarshal([]byte(value), &gitProvider); err != nil {
			return nil, fmt.Errorf("failed to unmarshal provider data for key %s: %w", key, err)
		}

		providers[key] = gitProvider
	}

	return providers, nil
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

func (r *GitRemoteReconciler) findObjectsForConfigMap(ctx context.Context, configMap client.Object) []reconcile.Request {
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
	configMap, ok := e.Object.(*v1.ConfigMap)
	if !ok {
		return false
	}
	return configMap.Namespace == r.Namespace && configMap.Name == "git-providers-endpoints"
}

func (r *GitRemoteReconciler) gitEndpointsConfigUpdate(e event.UpdateEvent) bool {
	configMap, ok := e.ObjectNew.(*v1.ConfigMap)
	if !ok {
		return false
	}
	return configMap.Namespace == r.Namespace && configMap.Name == "git-providers-endpoints"
}

func (r *GitRemoteReconciler) gitEndpointsConfigDeletion(e event.DeleteEvent) bool {
	configMap, ok := e.Object.(*v1.ConfigMap)
	if !ok {
		return false
	}
	return configMap.Namespace == r.Namespace && configMap.Name == "git-providers-endpoints"
}

const (
	secretRefField = ".spec.secretRef.name"
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
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForConfigMap),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}, configMapPredicates),
		).
		Complete(r)
}
