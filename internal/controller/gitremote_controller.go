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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kgiov1 "dams.kgio/kgio/api/v1"
)

// GitRemoteReconciler reconciles a GitRemote object
type GitRemoteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type SecretTypeMismatchError struct {
	FoundType string
}

func (e SecretTypeMismatchError) Error() string {
	return fmt.Sprintf("secret type is not BasicAuth, found: %s", e.FoundType)
}

// +kubebuilder:rbac:groups=kgio.dams.kgio,resources=gitremotes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kgio.dams.kgio,resources=gitremotes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kgio.dams.kgio,resources=gitremotes/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
func (r *GitRemoteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// Get the GitRemote Object
	var gitRemote kgiov1.GitRemote
	if err := r.Get(ctx, req.NamespacedName, &gitRemote); err != nil {
		// log.Log.Error(err, "unable to fetch GitRemote")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	gRNamespace := gitRemote.Namespace
	gRName := gitRemote.Name
	gitBaseDomainFQDN := gitRemote.Spec.GitBaseDomainFQDN
	log.Log.Info("Reconciling GitRemote " + gRNamespace + "/" + gRName)

	// Get the referenced Secret
	var secret corev1.Secret
	retrievedSecret := types.NamespacedName{Namespace: req.Namespace, Name: gitRemote.Spec.SecretRef.Name}
	if err := r.Get(ctx, retrievedSecret, &secret); err != nil {
		log.Log.Error(err, "["+gRNamespace+"/"+gRName+"] Secret not found with the name "+gitRemote.Spec.SecretRef.Name)
		gitRemote.Status.ConnexionStatus = kgiov1.Disconnected
		if err := r.Status().Update(ctx, &gitRemote); err != nil {
			log.Log.Error(err, "["+gRNamespace+"/"+gRName+"] Failed to update status")
		}
		return ctrl.Result{}, err
	}
	username := string(secret.Data["username"])
	log.Log.Info("[" + gRNamespace + "/" + gRName + "] Secret found, username : " + username)

	if gitRemote.Spec.TestAuthentication {
		log.Log.Info("[" + gRNamespace + "/" + gRName + "] Auth check on " + gitBaseDomainFQDN + " using the token associated to " + username)

		// Check if the referenced Secret is a basic-auth type
		if secret.Type != corev1.SecretTypeBasicAuth {
			err := SecretTypeMismatchError{FoundType: string(secret.Type)}
			log.Log.Error(err, err.Error())
			return ctrl.Result{}, err
		}

		// Get the username and password from the Secret
		gitRemote.Status.GitUserID = username
		PAToken := string(secret.Data["password"])

		// Determine Git provider based on GitBaseDomainFQDN
		var apiEndpoint string
		var forbiddenMessage kgiov1.GitRemoteConnexionStatus
		forbiddenMessage = kgiov1.Forbidden
		gitProvider := gitRemote.Spec.GitProvider
		switch gitProvider {
		case kgiov1.Github:
			apiEndpoint = "https://api." + gitBaseDomainFQDN + "/user"
		case kgiov1.Gitlab:
			apiEndpoint = "https://" + gitBaseDomainFQDN + "/api/v4/user"
			forbiddenMessage = "Forbidden: the 'read_user' (or 'read_api' for the old version), the 'read_repository' and the 'write_repository' permissions need to be granted"
		case kgiov1.Bitbucket:
			apiEndpoint = "https://api." + gitBaseDomainFQDN + "/2.0/user"
		default:
			err := fmt.Errorf("unsupported Git provider: %s", gitRemote.Spec.GitProvider)
			log.Log.Error(err, "["+gRNamespace+"/"+gRName+"] Unsupported Git provider")
			return ctrl.Result{}, err
		}
		log.Log.Info("[" + gRNamespace + "/" + gRName + "] Process authentication checking on this endpoint : " + apiEndpoint)

		// Perform Git provider authentication check
		client := &http.Client{}
		gitReq, err := http.NewRequest("GET", apiEndpoint, nil)
		if err != nil {
			log.Log.Error(err, "["+gRNamespace+"/"+gRName+"] Failed to create Git Auth Test request")
			return ctrl.Result{}, err
		}
		gitReq.Header.Add("Private-Token", PAToken)

		resp, err := client.Do(gitReq)
		if err != nil {
			log.Log.Error(err, "["+gRNamespace+"/"+gRName+"] Failed to perform the Git Auth Test request; cannot communicate with the remote Git server (%s)", gitProvider)
			return ctrl.Result{}, err
		}
		defer resp.Body.Close()

		// Check the response status code
		if resp.StatusCode == http.StatusOK {
			// Authentication successful
			gitRemote.Status.ConnexionStatus = kgiov1.Connected
			gitRemote.Status.LastAuthTime = metav1.Now()
			log.Log.Info("[" + gRNamespace + "/" + gRName + "] Auth successed: " + username + " connected")
		} else if resp.StatusCode == http.StatusUnauthorized {
			// Unauthorized: bad credentials
			gitRemote.Status.ConnexionStatus = kgiov1.Unauthorized
			log.Log.Info("[" + gRNamespace + "/" + gRName + "] Auth failed: Unauthorized")
		} else if resp.StatusCode == http.StatusForbidden {
			// Forbidden : Not enough permission
			gitRemote.Status.ConnexionStatus = forbiddenMessage
			log.Log.Info("[" + gRNamespace + "/" + gRName + "] Auth failed: " + string(forbiddenMessage))
		} else if resp.StatusCode == http.StatusInternalServerError {
			// Server error: a server error happened
			gitRemote.Status.ConnexionStatus = kgiov1.ServerError
			log.Log.Info("[" + gRNamespace + "/" + gRName + "] Auth failed: " + gitBaseDomainFQDN + " returns a Server Error")
		} else {
			// Handle other status codes if needed
			log.Log.Info("[" + gRNamespace + "/" + gRName + "] Auth failed: Unexpected error occured")
			gitRemote.Status.ConnexionStatus = kgiov1.UnexpectedStatus
			err := fmt.Errorf("["+gRNamespace+"/"+gRName+"] Unexpected response status code: %d", resp.StatusCode)
			log.Log.Error(err, "["+gRNamespace+"/"+gRName+"] Unexpected response from "+string(gitProvider))
			if err := r.Status().Update(ctx, &gitRemote); err != nil {
				log.Log.Error(err, "["+gRNamespace+"/"+gRName+"] Failed to update status")
			}
			return ctrl.Result{}, err
		}
	}

	// Update the status of GitRemote
	if err := r.Status().Update(ctx, &gitRemote); err != nil {
		log.Log.Error(err, "["+gRNamespace+"/"+gRName+"] Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&kgiov1.GitRemote{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSecret),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
