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

package v1alpha4

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
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
