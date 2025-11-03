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

package v1beta2

import (
	"context"
	"fmt"
	"regexp"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	syngitv1beta2 "github.com/syngit-org/syngit/pkg/api/v1beta2"
)

// nolint:unused
// log is for logging in this package.
var remotesyncerlog = logf.Log.WithName("remotesyncer-resource")

// SetupRemoteSyncerWebhookWithManager registers the webhook for RemoteSyncer in the manager.
func SetupRemoteSyncerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&syngitv1beta2.RemoteSyncer{}).
		WithValidator(&RemoteSyncerCustomValidator{}).
		Complete()
}

type RemoteSyncerCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &RemoteSyncerCustomValidator{}

// Validate validates the RemoteSyncerSpec
func validateRemoteSyncerSpec(r *syngitv1beta2.RemoteSyncerSpec) field.ErrorList {
	var errors field.ErrorList

	// Validate DefaultUserBind based on DefaultUnauthorizedUserMode
	if r.DefaultUnauthorizedUserMode == syngitv1beta2.Block && r.DefaultRemoteUserRef != nil {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("defaultRemoteUserRef"), r.DefaultRemoteUserRef, "should not be set when defaultUnauthorizedUserMode is set to \"Block\""))
	} else if r.DefaultUnauthorizedUserMode == syngitv1beta2.UseDefaultUser && r.DefaultRemoteUserRef == nil {
		errors = append(errors, field.Required(field.NewPath("spec").Child("defaultRemoteUserRef"), "must be set when defaultUnauthorizedUserMode is set to \"UseDefaultUser\""))
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
	if r.PushMode == syngitv1beta2.SameBranch && r.DefaultBranch == "" {
		errors = append(errors, field.Required(field.NewPath("spec").Child("defaultBranch"), "must be set when defaultBranch is set to \"SameBranch\""))
	}

	// Validate that DefaultBranch exists if DefaultUnauthorizedUser uses a default user
	if r.DefaultUnauthorizedUserMode != syngitv1beta2.Block && r.DefaultBranch == "" {
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

func validateRemoteSyncer(remoteSyncer *syngitv1beta2.RemoteSyncer) error {
	var allErrs field.ErrorList
	if err := validateRemoteSyncerSpec(&remoteSyncer.Spec); err != nil {
		allErrs = append(allErrs, err...)
	}
	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "syngit.io", Kind: "RemoteSyncer"},
		remoteSyncer.Name, allErrs)
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type RemoteSyncer.
func (v *RemoteSyncerCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	remotesyncer, ok := obj.(*syngitv1beta2.RemoteSyncer)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteSyncer object but got %T", obj)
	}
	remotesyncerlog.Info("Validation for RemoteSyncer upon creation", "name", remotesyncer.GetName())

	return nil, validateRemoteSyncer(remotesyncer)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type RemoteSyncer.
func (v *RemoteSyncerCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	remotesyncer, ok := newObj.(*syngitv1beta2.RemoteSyncer)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteSyncer object for the newObj but got %T", newObj)
	}
	remotesyncerlog.Info("Validation for RemoteSyncer upon update", "name", remotesyncer.GetName())

	return nil, validateRemoteSyncer(remotesyncer)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type RemoteSyncer.
func (v *RemoteSyncerCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	remotesyncer, ok := obj.(*syngitv1beta2.RemoteSyncer)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteSyncer object but got %T", obj)
	}
	remotesyncerlog.Info("Validation for RemoteSyncer upon deletion", "name", remotesyncer.GetName())

	return nil, nil
}
