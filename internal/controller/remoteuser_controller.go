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
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	syngit "syngit.io/syngit/api/v3alpha3"
)

// RemoteUserReconciler reconciles a RemoteUser object
type RemoteUserReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Namespace string
}

func (r *RemoteUserReconciler) setServerConfiguration(ctx context.Context, remoteUser *syngit.RemoteUser) (syngit.GitServerConfiguration, error) {

	gpc := &syngit.GitServerConfiguration{
		Inherited:              false,
		AuthenticationEndpoint: "",
		CaBundle:               "",
		InsecureSkipTlsVerify:  false,
	}

	// STEP 1 : Check the config map ref
	var cm corev1.ConfigMap
	if remoteUser.Spec.CustomGitServerConfigRef.Name != "" {
		// It is defined in the RemoteUser object
		namespacedName := types.NamespacedName{Namespace: remoteUser.Namespace, Name: remoteUser.Spec.CustomGitServerConfigRef.Name}
		if err := r.Get(ctx, namespacedName, &cm); err != nil {
			remoteUser.Status.ConnexionStatus.Status = syngit.GitConfigNotFound
			remoteUser.Status.ConnexionStatus.Details = "ConfigMap name: " + remoteUser.Spec.CustomGitServerConfigRef.Name
			return *gpc, err
		}
	} else {
		// It is not defined in the RemoteUser object -> look for the default configmap of the operator
		namespacedName := types.NamespacedName{Namespace: r.Namespace, Name: remoteUser.Spec.GitBaseDomainFQDN}
		if err := r.Get(ctx, namespacedName, &cm); err != nil {
			remoteUser.Status.ConnexionStatus.Status = syngit.GitConfigNotFound
			remoteUser.Status.ConnexionStatus.Details = "Configuration reference not found in the current RemoteUser; ConfigMap " + remoteUser.Spec.GitBaseDomainFQDN + " in the namespace of the operator not found as well"
			return *gpc, err
		}
		gpc.Inherited = true
	}

	// STEP 2 : Build the GitServerConfiguration

	// Parse the ConfigMap
	serverConf, err := parseConfigMap(cm)
	if err != nil {
		remoteUser.Status.ConnexionStatus.Status = syngit.GitConfigParseError
		remoteUser.Status.ConnexionStatus.Details = err.Error()
		return *gpc, err
	}

	if remoteUser.Spec.InsecureSkipTlsVerify && remoteUser.Spec.InsecureSkipTlsVerify != serverConf.InsecureSkipTlsVerify {
		serverConf.InsecureSkipTlsVerify = remoteUser.Spec.InsecureSkipTlsVerify
	}

	*gpc = serverConf

	return *gpc, nil
}

// +kubebuilder:rbac:groups=syngit.syngit.io,resources=remoteusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syngit.syngit.io,resources=remoteusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=syngit.syngit.io,resources=remoteusers/finalizers,verbs=update
// +kubebuilder:rbac:groups=corev1,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=corev1,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=corev1,resources=events,verbs=create;patch

func (r *RemoteUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// Get the RemoteUser Object
	var remoteUser syngit.RemoteUser
	if err := r.Get(ctx, req.NamespacedName, &remoteUser); err != nil {
		// does not exists -> deleted
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	gRNamespace := remoteUser.Namespace
	gRName := remoteUser.Name

	var prefixMsg = "[" + gRNamespace + "/" + gRName + "]"
	log.Log.Info(prefixMsg + " Reconciling request received")

	condition := &v1.Condition{
		LastTransitionTime: v1.Now(),
		Type:               "NotReady",
		Status:             "False",
	}

	// Get the referenced Secret
	var secret corev1.Secret
	namespacedNameSecret := types.NamespacedName{Namespace: req.Namespace, Name: remoteUser.Spec.SecretRef.Name}
	if err := r.Get(ctx, namespacedNameSecret, &secret); err != nil {
		remoteUser.Status.SecretBoundStatus = syngit.SecretNotFound
		remoteUser.Status.ConnexionStatus.Status = ""

		condition.Reason = "SecretNotFound"
		condition.Message = string(syngit.SecretNotFound)
		err = r.updateStatus(ctx, &remoteUser, *condition)

		return ctrl.Result{}, err
	}
	remoteUser.Status.SecretBoundStatus = syngit.SecretBound
	username := string(secret.Data["username"])

	// Update configuration
	gpc, err := r.setServerConfiguration(ctx, &remoteUser)
	if err != nil {

		condition.Reason = "RemoteUserServerConfigurationError"
		condition.Message = err.Error()
		errUpdate := r.updateStatus(ctx, &remoteUser, *condition)

		return ctrl.Result{}, errUpdate
	}

	condition.Type = "Ready"
	condition.Status = "True"

	remoteUser.Status.GitServerConfiguration = gpc
	condition.Reason = "RemoteUserServerConfigurationAssigned"
	condition.Message = "The git remote server configuration has been assigned to this object"
	errUpdate := r.updateStatus(ctx, &remoteUser, *condition)
	if errUpdate != nil {
		return ctrl.Result{}, errUpdate
	}

	if remoteUser.Spec.TestAuthentication {
		condition.Type = "NotReady"
		condition.Status = "False"

		// Check if the referenced Secret is a basic-auth type
		if secret.Type != corev1.SecretTypeBasicAuth {

			remoteUser.Status.SecretBoundStatus = syngit.SecretWrongType

			condition.Reason = "SecretWrongType"
			condition.Message = string(syngit.SecretWrongType)
			errUpdate := r.updateStatus(ctx, &remoteUser, *condition)

			return ctrl.Result{}, errUpdate
		}

		// Get the username and password from the Secret
		remoteUser.Status.GitUser = username
		PAToken := string(secret.Data["password"])

		// If test auth -> the endpoint must exists
		authenticationEndpoint := gpc.AuthenticationEndpoint
		if authenticationEndpoint == "" {
			errMsg := ""
			if gpc.Inherited {
				errMsg = "git provider not found in the " + remoteUser.Spec.GitBaseDomainFQDN + " ConfigMap in the namespace of the operator"
			} else {
				errMsg = "git provider not found in the " + remoteUser.Spec.CustomGitServerConfigRef.Name + " ConfigMap"
			}
			remoteUser.Status.ConnexionStatus.Status = syngit.GitUnsupported
			remoteUser.Status.ConnexionStatus.Details = errMsg

			condition.Reason = "WrongRemoteUserServerConfiguration"
			condition.Message = errMsg
			errUpdate := r.updateStatus(ctx, &remoteUser, *condition)

			return ctrl.Result{}, errUpdate
		}

		// Perform Git provider authentication check
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: gpc.InsecureSkipTlsVerify,
			},
		}
		if !gpc.InsecureSkipTlsVerify && gpc.CaBundle != "" {
			caCertPool := x509.NewCertPool()
			if ok := caCertPool.AppendCertsFromPEM([]byte(gpc.CaBundle)); !ok {
				remoteUser.Status.ConnexionStatus.Status = syngit.GitConfigParseError
				remoteUser.Status.ConnexionStatus.Details = "x509 cert pool maker failed"

				condition.Reason = "RemoteUserServerCertificateMalformed"
				condition.Message = remoteUser.Status.ConnexionStatus.Details
				errUpdate := r.updateStatus(ctx, &remoteUser, *condition)

				return ctrl.Result{}, errUpdate
			}
			transport.TLSClientConfig.RootCAs = caCertPool
		}
		httpClient := &http.Client{
			Transport: transport,
		}
		gitReq, err := http.NewRequest("GET", authenticationEndpoint, nil)
		if err != nil {
			remoteUser.Status.ConnexionStatus.Status = syngit.GitServerError
			remoteUser.Status.ConnexionStatus.Details = "Internal operator error : cannot create the http request " + err.Error()

			condition.Reason = "RemoteUserServerError"
			condition.Message = remoteUser.Status.ConnexionStatus.Details
			errUpdate := r.updateStatus(ctx, &remoteUser, *condition)

			return ctrl.Result{}, errUpdate
		}

		// For gitlab
		gitReq.Header.Add("Private-Token", PAToken)

		// If needed because there is a conflict between github and bitbucket
		// They both uses the same key to authenticate but not the same value
		if strings.Contains(authenticationEndpoint, "github.com") {
			// For github
			gitReq.Header.Set("Authorization", "token "+PAToken)
		} else if strings.Contains(authenticationEndpoint, "bitbucket.org") {
			// For bitbucket
			bitbucketAuth := base64.StdEncoding.EncodeToString([]byte(username + ":" + PAToken))
			gitReq.Header.Set("Authorization", "Basic "+bitbucketAuth)
		}

		resp, err := httpClient.Do(gitReq)
		if err != nil {
			remoteUser.Status.ConnexionStatus.Status = syngit.GitServerError
			remoteUser.Status.ConnexionStatus.Details = "Internal operator error : the request cannot be processed " + err.Error()

			condition.Reason = "RemoteUserServerError"
			condition.Message = remoteUser.Status.ConnexionStatus.Details
			errUpdate := r.updateStatus(ctx, &remoteUser, *condition)

			return ctrl.Result{}, errUpdate
		}
		defer resp.Body.Close()

		remoteUser.Status.ConnexionStatus.Details = ""

		condition.Type = "AuthFailed"

		// Check the response status code
		if resp.StatusCode == http.StatusOK {
			// Authentication successful
			remoteUser.Status.ConnexionStatus.Status = syngit.GitConnected
			remoteUser.Status.LastAuthTime = v1.Now()

			condition.Type = "AuthSucceeded"
			condition.Status = "True"
			condition.Reason = "RemoteUserUserConnected"
			condition.Message = "Successfully logged to the remote git server with the git user specified"
			r.Recorder.Event(&remoteUser, "Normal", "Connected", "Auth succeeded")
		} else if resp.StatusCode == http.StatusUnauthorized {
			// Unauthorized: bad credentials
			remoteUser.Status.ConnexionStatus.Status = syngit.GitUnauthorized

			condition.Reason = "RemoteUserUserUnauthorized"
			condition.Message = string(remoteUser.Status.ConnexionStatus.Status)
			r.Recorder.Event(&remoteUser, "Warning", "AuthFailed", "Auth failed - unauthorized")
		} else if resp.StatusCode == http.StatusForbidden {
			// Forbidden : Not enough permission
			remoteUser.Status.ConnexionStatus.Status = syngit.GitForbidden

			condition.Reason = "RemoteUserUserForbidden"
			condition.Message = string(remoteUser.Status.ConnexionStatus.Status)
			r.Recorder.Event(&remoteUser, "Warning", "AuthFailed", "Auth failed - forbidden")
		} else if resp.StatusCode == http.StatusInternalServerError {
			// Server error: a server error happened
			remoteUser.Status.ConnexionStatus.Status = syngit.GitServerError

			condition.Reason = "RemoteUserServerError"
			condition.Message = string(remoteUser.Status.ConnexionStatus.Status)
			r.Recorder.Event(&remoteUser, "Warning", "AuthFailed", "Auth failed - server error")
		} else {
			// Handle other status codes if needed
			remoteUser.Status.ConnexionStatus.Status = syngit.GitUnexpectedStatus

			condition.Reason = "RemoteUserServerError"
			condition.Message = string(remoteUser.Status.ConnexionStatus.Status)
			r.Recorder.Event(&remoteUser, "Warning", "AuthFailed",
				fmt.Sprintf("Auth failed - unexpected response - %s", resp.Status))
		}
	}

	// Update the status of RemoteUser
	r.updateStatus(ctx, &remoteUser, *condition)

	return ctrl.Result{}, nil
}

func parseConfigMap(configMap corev1.ConfigMap) (syngit.GitServerConfiguration, error) {
	gitServerConf := &syngit.GitServerConfiguration{}
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

func (r *RemoteUserReconciler) updateConditions(remoteUser syngit.RemoteUser, condition v1.Condition) []v1.Condition {
	added := false
	var conditions []v1.Condition
	for _, cond := range remoteUser.Status.Conditions {
		if cond.Type == condition.Type {
			conditions = append(conditions, condition)
			added = true
		} else {
			conditions = append(conditions, cond)
		}
	}
	if !added {
		conditions = append(conditions, condition)
	}
	return conditions
}

func (r *RemoteUserReconciler) updateStatus(ctx context.Context, remoteUser *syngit.RemoteUser, condition v1.Condition) error {
	conditions := r.updateConditions(*remoteUser, condition)

	remoteUser.Status.Conditions = conditions
	if err := r.Status().Update(ctx, remoteUser); err != nil {
		return err
	}
	return nil
}

func (r *RemoteUserReconciler) findObjectsForSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	attachedRemoteUsers := &syngit.RemoteUserList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(secretRefField, secret.GetName()),
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

func (r *RemoteUserReconciler) findObjectsForGitProviderConfig(ctx context.Context, configMap client.Object) []reconcile.Request {
	attachedRemoteUsers := &syngit.RemoteUserList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(gitProviderConfigRefField, configMap.GetName()),
		Namespace:     configMap.GetNamespace(),
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

func (r *RemoteUserReconciler) findObjectsForRootConfigMap(ctx context.Context, configMap client.Object) []reconcile.Request {
	attachedRemoteUsers := &syngit.RemoteUserList{}
	listOps := &client.ListOptions{}
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

func (r *RemoteUserReconciler) gitEndpointsConfigCreation(e event.CreateEvent) bool {
	configMap, ok := e.Object.(*corev1.ConfigMap)
	if !ok {
		return false
	}
	return configMap.Namespace == r.Namespace && strings.Contains(configMap.Name, ".")
}

func (r *RemoteUserReconciler) gitEndpointsConfigUpdate(e event.UpdateEvent) bool {
	configMap, ok := e.ObjectNew.(*corev1.ConfigMap)
	if !ok {
		return false
	}
	return configMap.Namespace == r.Namespace && strings.Contains(configMap.Name, ".")
}

func (r *RemoteUserReconciler) gitEndpointsConfigDeletion(e event.DeleteEvent) bool {
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
func (r *RemoteUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &syngit.RemoteUser{}, secretRefField, func(rawObj client.Object) []string {
		// Extract the Secret name from the RemoteUser Spec, if one is provided
		remoteUser := rawObj.(*syngit.RemoteUser)
		if remoteUser.Spec.SecretRef.Name == "" {
			return nil
		}
		return []string{remoteUser.Spec.SecretRef.Name}
	}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &syngit.RemoteUser{}, gitProviderConfigRefField, func(rawObj client.Object) []string {
		// Extract the ConfigMap name from the RemoteUser Spec, if one is provided
		remoteUser := rawObj.(*syngit.RemoteUser)
		if remoteUser.Spec.CustomGitServerConfigRef.Name == "" {
			return nil
		}
		return []string{remoteUser.Spec.CustomGitServerConfigRef.Name}
	}); err != nil {
		return err
	}

	// Recorder to manage events
	recorder := mgr.GetEventRecorderFor("remoteuser-controller")
	r.Recorder = recorder

	managerNamespace := os.Getenv("MANAGER_NAMESPACE")
	r.Namespace = managerNamespace

	configMapPredicates := predicate.Funcs{
		CreateFunc: r.gitEndpointsConfigCreation,
		UpdateFunc: r.gitEndpointsConfigUpdate,
		DeleteFunc: r.gitEndpointsConfigDeletion,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&syngit.RemoteUser{}).
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
