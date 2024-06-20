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

package v1alpha2

import (
	"context"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var remoteuserlog = logf.Log.WithName("remoteuser-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *RemoteUser) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-syngit-damsien-fr-v1alpha2-remoteuser,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.damsien.fr,resources=remoteusers,verbs=create;update,versions=v1alpha2,name=vremoteuser.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/reconcile-syngit-remoteuser-owner,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.damsien.fr,resources=remoteusers,verbs=create,versions=v1alpha2,admissionReviewVersions=v1,name=vremoteusers-owner.kb.io

var _ webhook.Validator = &RemoteUser{}

// Validate validates the RemoteUserSpec
func (r *RemoteUserSpec) ValidateRemoteUserSpec() field.ErrorList {
	var errors field.ErrorList

	return errors
}

func (r *RemoteUser) ValidateRemoteUser() error {
	var allErrs field.ErrorList
	if err := r.Spec.ValidateRemoteUserSpec(); err != nil {
		allErrs = append(allErrs, err...)
	}
	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		r.GroupVersionKind().GroupKind(),
		r.Name, allErrs)
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteUser) ValidateCreate() (admission.Warnings, error) {
	remoteuserlog.Info("validate create", "name", r.Name)

	return nil, r.ValidateRemoteUser()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteUser) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	remoteuserlog.Info("validate update", "name", r.Name)

	return nil, r.ValidateRemoteUser()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteUser) ValidateDelete() (admission.Warnings, error) {
	remoteuserlog.Info("validate delete", "name", r.Name)

	// Nothing to validate
	return nil, nil
}

/*
	Handle webhook and get kubernetes user id
*/

type RemoteUserWebhookHandler struct {
	Client  client.Client
	Decoder *admission.Decoder
}

func (ruwh *RemoteUserWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	ru := &RemoteUser{}
	err := ruwh.Decoder.Decode(req, ru)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if !ru.Spec.OwnRemoteUserBinding {
		return admission.Allowed("This object does not own a RemoteUserBinding")
	}

	username := req.DeepCopy().UserInfo.Username
	objRef := corev1.ObjectReference{Name: ru.Name}
	name := rubPrefix + username
	rub := &RemoteUserBinding{}
	webhookNamespacedName := &types.NamespacedName{
		Name:      name,
		Namespace: req.Namespace,
	}
	err = ruwh.Client.Get(ctx, *webhookNamespacedName, rub)
	if err != nil {
		// The RemoteUserBinding does not exists yet
		rub.Name = name
		rub.Namespace = req.Namespace

		subject := &rbacv1.Subject{
			Kind: "User",
			Name: username,
		}
		rub.Spec.Subject = *subject

		remoteRefs := make([]corev1.ObjectReference, 0)
		remoteRefs = append(remoteRefs, objRef)
		rub.Spec.RemoteRefs = remoteRefs

		createErr := ruwh.Client.Create(ctx, rub)
		if createErr != nil {
			return admission.Errored(http.StatusInternalServerError, createErr)
		}
	} else {
		// The RemoteUserBinding already exists

		remoteRefs := rub.DeepCopy().Spec.RemoteRefs
		remoteRefs = append(remoteRefs, objRef)
		rub.Spec.RemoteRefs = remoteRefs

		updateErr := ruwh.Client.Update(ctx, rub)
		if updateErr != nil {
			return admission.Errored(http.StatusInternalServerError, updateErr)
		}
	}

	return admission.Allowed("This object owns the " + name + " RemoteUserBinding")
}
