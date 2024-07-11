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

package v2alpha2

import (
	"regexp"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

//+kubebuilder:webhook:path=/validate-syngit-syngit-io-v2alpha2-remotesyncer,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.syngit.io,resources=remotesyncers,verbs=create;update,versions=v2alpha2,name=vremotesyncer.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &RemoteSyncer{}

// Validate validates the RemoteSyncerSpec
func (r *RemoteSyncerSpec) ValidateRemoteSyncerSpec() field.ErrorList {
	var errors field.ErrorList

	// Validate DefaultUserBind based on DefaultUnauthorizedUserMode
	if r.DefaultUnauthorizedUserMode == Block && r.DefaultUserBind != nil {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("defaultUserBind"), r.DefaultUserBind, "should not be set when defaultUnauthorizedUserMode is set to \"Block\""))
	} else if r.DefaultUnauthorizedUserMode == UserDefaultUserBind && r.DefaultUserBind == nil {
		errors = append(errors, field.Required(field.NewPath("spec").Child("defaultUserBind"), "should be set when defaultUnauthorizedUserMode is set to \"UseDefaultUserBind\""))
	}

	// Validate DefaultBlockAppliedMessage only exists if CommitProcess is set to ApplyCommit
	if r.DefaultBlockAppliedMessage != "" && r.CommitProcess != "CommitApply" {
		errors = append(errors, field.Forbidden(field.NewPath("spec").Child("defaultBlockAppliedMessage"), "should not be set if .spec.commitProcess is not set to \"CommitApply\""))
	}

	// Validate that CommitProcess is either CommitApply or CommitOnly
	if r.CommitProcess != "CommitOnly" && r.CommitProcess != "CommitApply" {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("commitProcess"), r.CommitProcess, "should be set to \"CommitApply\" or \"CommitOnly\""))
	}

	// Validate Git URI
	gitURIPattern := regexp.MustCompile(`^(https?|git|ssh|ftps?|rsync)\://[^ ]+$`)
	if !gitURIPattern.MatchString(r.RemoteRepository) {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("remoteRepository"), r.RemoteRepository, "invalid Git URI"))
	}

	// Validate the ExcludedFields to ensure that it is a YAML path
	for _, fieldPath := range r.ExcludedFields {
		if !isValidYAMLPath(fieldPath) {
			errors = append(errors, field.Invalid(field.NewPath("spec").Child("excludedFields"), fieldPath, "must be a valid YAML path. Regex : "+`^([a-zA-Z0-9_./:-]*(\[[a-zA-Z0-9_*./:-]*\])?)*$`))
		}
	}

	return errors
}

// isValidYAMLPath checks if the given string is a valid YAML path
func isValidYAMLPath(path string) bool {
	// Regular expression to match a valid YAML path
	yamlPathRegex := regexp.MustCompile(`^([a-zA-Z0-9_./:-]*(\[[a-zA-Z0-9_*./:-]*\])?)*$`)
	return yamlPathRegex.MatchString(path)
}

func (r *RemoteSyncerSpec) searchForDuplicates(gvrns []GroupVersionResourceName) []*schema.GroupVersionResource {
	seen := make(map[string]bool)
	duplicates := make([]*schema.GroupVersionResource, 0)

	for _, item := range gvrns {
		if _, ok := seen[item.GroupVersionResource.String()]; ok {
			duplicates = append(duplicates, item.GroupVersionResource)
		}
		seen[item.GroupVersionResource.String()] = true
	}

	return duplicates
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
