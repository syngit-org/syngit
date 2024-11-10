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

package v1beta1

import (
	"regexp"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var remotesyncerlog = logf.Log.WithName("remotesyncer-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *RemoteSyncer) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-syngit-syngit-io-v1beta1-remotesyncer,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.syngit.io,resources=remotesyncers,verbs=create;update,versions=v1beta1,name=vremotesyncer.v1beta1.syngit.io,admissionReviewVersions=v1

var _ webhook.Validator = &RemoteSyncer{}

// Validate validates the RemoteSyncerSpec
func (r *RemoteSyncerSpec) ValidateRemoteSyncerSpec() field.ErrorList {
	var errors field.ErrorList

	// Validate DefaultUserBind based on DefaultUnauthorizedUserMode
	if r.DefaultUnauthorizedUserMode == Block && r.DefaultRemoteUserRef != nil {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("defaultUser"), r.DefaultRemoteUserRef, "should not be set when defaultUnauthorizedUserMode is set to \"Block\""))
	} else if r.DefaultUnauthorizedUserMode == UseDefaultUser && r.DefaultRemoteUserRef == nil {
		errors = append(errors, field.Required(field.NewPath("spec").Child("defaultUser"), "must be set when defaultUnauthorizedUserMode is set to \"UseDefaultUser\""))
	}

	// Validate DefaultBlockAppliedMessage only exists if ProcessMode is set to CommitOnly
	if r.DefaultBlockAppliedMessage != "" && r.ProcessMode != "CommitOnly" {
		errors = append(errors, field.Forbidden(field.NewPath("spec").Child("defaultBlockAppliedMessage"), "should not be set if processMode is not set to \"CommitOnly\""))
	}

	// Validate that ProcessMode is either CommitApply or CommitOnly
	if r.ProcessMode != "CommitOnly" && r.ProcessMode != "CommitApply" {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("processMode"), r.ProcessMode, "must be set to \"CommitApply\" or \"CommitOnly\""))
	}

	// Validate Git URI
	gitURIPattern := regexp.MustCompile(`^(https?|git)\://[^ ]+$`)
	if !gitURIPattern.MatchString(r.RemoteRepository) {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("remoteRepository"), r.RemoteRepository, "invalid Git URI"))
	}

	// Validate the ExcludedFields to ensure that it is a YAML path
	for _, fieldPath := range r.ExcludedFields {
		if !isValidYAMLPath(fieldPath) {
			errors = append(errors, field.Invalid(field.NewPath("spec").Child("excludedFields"), fieldPath, "must be a valid YAML path. Regex : "+`^([a-zA-Z0-9_./:-]*(\[[a-zA-Z0-9_*./:-]*\])?)*$`))
		}
	}

	// Validate that DefaultBranch exists if PushMode is set to "SameBranch"
	if r.PushMode == SameBranch && r.DefaultBranch == "" {
		errors = append(errors, field.Required(field.NewPath("spec").Child("defaultBranch"), "must be set when defaultBranch is set to \"SameBranch\""))
	}

	// Validate that DefaultBranch exists if DefaultUnauthorizedUser uses a default user
	if r.DefaultUnauthorizedUserMode != Block && r.DefaultBranch == "" {
		errors = append(errors, field.Required(field.NewPath("spec").Child("defaultBranch"), "must be set when the defaultUnauthorizedUserMode is set to UseDefaultUser"))
	}

	return errors
}

// isValidYAMLPath checks if the given string is a valid YAML path
func isValidYAMLPath(path string) bool {
	// Regular expression to match a valid YAML path
	yamlPathRegex := regexp.MustCompile(`^([a-zA-Z0-9_./:-]*(\[[a-zA-Z0-9_*./:-]*\])?)*$`)
	return yamlPathRegex.MatchString(path)
}

func (r *RemoteSyncer) ValidateRemoteSyncer() error {
	var allErrs field.ErrorList
	if err := r.Spec.ValidateRemoteSyncerSpec(); err != nil {
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
func (r *RemoteSyncer) ValidateCreate() (admission.Warnings, error) {
	remotesyncerlog.Info("validate create", "name", r.Name)

	return nil, r.ValidateRemoteSyncer()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteSyncer) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	remotesyncerlog.Info("validate update", "name", r.Name)

	return nil, r.ValidateRemoteSyncer()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteSyncer) ValidateDelete() (admission.Warnings, error) {
	remotesyncerlog.Info("validate delete", "name", r.Name)

	// Nothing to validate
	return nil, nil
}
