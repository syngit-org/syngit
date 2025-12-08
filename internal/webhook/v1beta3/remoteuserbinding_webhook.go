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

package v1beta3

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	syngitv1beta3 "github.com/syngit-org/syngit/pkg/api/v1beta3"
)

// nolint:unused
// log is for logging in this package.
var remoteuserbindinglog = logf.Log.WithName("remoteuserbinding-resource")

// SetupRemoteUserBindingWebhookWithManager registers the webhook for RemoteUserBinding in the manager.
func SetupRemoteUserBindingWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&syngitv1beta3.RemoteUserBinding{}).
		WithValidator(&RemoteUserBindingCustomValidator{}).
		Complete()
}

type RemoteUserBindingCustomValidator struct {
}

var _ webhook.CustomValidator = &RemoteUserBindingCustomValidator{}

// Validate validates the RemoteSyncerSpec
func validateRemoteUserBindingSpec(r *syngitv1beta3.RemoteUserBindingSpec) field.ErrorList {
	var errors field.ErrorList

	// Validate that no namespaces are referenced
	for i, remoteUserRef := range r.RemoteUserRefs {
		if remoteUserRef.Namespace != "" {
			errors = append(errors, field.Invalid(field.NewPath("spec").Child("remoteUserRefs").Index(i), r.RemoteUserRefs[i].Namespace, "should not be set as it is not supported in this version of syngit"))
		}
	}
	for i, remoteTargetRef := range r.RemoteTargetRefs {
		if remoteTargetRef.Namespace != "" {
			errors = append(errors, field.Invalid(field.NewPath("spec").Child("remoteTargetRefs").Index(i), r.RemoteTargetRefs[i].Namespace, "should not be set as it is not supported in this version of syngit"))
		}
	}

	return errors
}

func validateRemoteUserBinding(remoteUserBinding *syngitv1beta3.RemoteUserBinding) error {
	var allErrs field.ErrorList
	if err := validateRemoteUserBindingSpec(&remoteUserBinding.Spec); err != nil {
		allErrs = append(allErrs, err...)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "syngit.io", Kind: "RemoteUserBinding"},
		remoteUserBinding.Name, allErrs)
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type RemoteUserBinding.
func (v *RemoteUserBindingCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	remoteuserbinding, ok := obj.(*syngitv1beta3.RemoteUserBinding)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteUserBinding object but got %T", obj)
	}
	remoteuserbindinglog.Info("Validation for RemoteUserBinding upon creation", "name", remoteuserbinding.GetName())

	return nil, validateRemoteUserBinding(remoteuserbinding)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type RemoteUserBinding.
func (v *RemoteUserBindingCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	remoteuserbinding, ok := newObj.(*syngitv1beta3.RemoteUserBinding)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteUserBinding object for the newObj but got %T", newObj)
	}
	remoteuserbindinglog.Info("Validation for RemoteUserBinding upon update", "name", remoteuserbinding.GetName())

	return nil, validateRemoteUserBinding(remoteuserbinding)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type RemoteUserBinding.
func (v *RemoteUserBindingCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	remoteuserbinding, ok := obj.(*syngitv1beta3.RemoteUserBinding)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteUserBinding object but got %T", obj)
	}
	remoteuserbindinglog.Info("Validation for RemoteUserBinding upon deletion", "name", remoteuserbinding.GetName())

	return nil, nil
}
