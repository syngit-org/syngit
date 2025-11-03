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
	"regexp"
	"slices"

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
var remotesyncerlog = logf.Log.WithName("remotesyncer-resource")

// SetupRemoteSyncerWebhookWithManager registers the webhook for RemoteSyncer in the manager.
func SetupRemoteSyncerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&syngitv1beta3.RemoteSyncer{}).
		WithValidator(&RemoteSyncerCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-syngit-io-v1beta3-remotesyncer,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remotesyncers,verbs=create;update,versions=v1beta3,name=vremotesyncer-v1beta3.kb.io,admissionReviewVersions=v1

type RemoteSyncerCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &RemoteSyncerCustomValidator{}

// Validate validates the RemoteSyncerSpec
func validateRemoteSyncerSpec(r *syngitv1beta3.RemoteSyncerSpec) field.ErrorList {
	var errors field.ErrorList

	// Validate DefaultRemoteUserRef based on DefaultUnauthorizedUserMode
	if r.DefaultUnauthorizedUserMode == syngitv1beta3.Block && r.DefaultRemoteUserRef != nil {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("defaultRemoteUserRef"), r.DefaultRemoteUserRef, "should not be set when defaultUnauthorizedUserMode is set to \"Block\""))
	} else if r.DefaultUnauthorizedUserMode == syngitv1beta3.UseDefaultUser && r.DefaultRemoteUserRef == nil {
		errors = append(errors, field.Required(field.NewPath("spec").Child("defaultRemoteUserRef"), "must be set when defaultUnauthorizedUserMode is set to \"UseDefaultUser\""))
	}

	// Validate DefaultRemoteUserRef and DefaultRemoteTargetRef
	if r.DefaultRemoteUserRef != nil && r.DefaultRemoteTargetRef == nil {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("defaultRemoteTargetRef"), r.DefaultRemoteTargetRef, "should be set when defaultRemoteUserRef is set"))
	}
	if r.DefaultRemoteUserRef == nil && r.DefaultRemoteTargetRef != nil {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("defaultRemoteUserRef"), r.DefaultRemoteUserRef, "should be set when defaultRemoteTargetRef is set"))
	}

	// Validate DefaultBlockAppliedMessage only exists if Strategy is set to CommitOnly
	if r.DefaultBlockAppliedMessage != "" && r.Strategy != syngitv1beta3.CommitOnly {
		errors = append(errors, field.Forbidden(field.NewPath("spec").Child("defaultBlockAppliedMessage"), fmt.Sprintf("should not be set if strategy is not set to \"%s\"", syngitv1beta3.CommitOnly)))
	}

	// Validate that Strategy is either CommitApply or CommitOnly
	if r.Strategy != syngitv1beta3.CommitOnly && r.Strategy != syngitv1beta3.CommitApply {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("strategy"), r.Strategy, fmt.Sprintf("must be set to \"%s\" or \"%s\"", syngitv1beta3.CommitApply, syngitv1beta3.CommitOnly)))
	}

	// Validate Git URI
	gitURIPattern := regexp.MustCompile(`^(https?|git)://((([a-zA-Z0-9-]+\.)+[a-zA-Z]{2,})|(\d{1,3}(\.\d{1,3}){3}))(:\d+)?(/.+)\.git$`)
	if !gitURIPattern.MatchString(r.RemoteRepository) {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("remoteRepository"), r.RemoteRepository, "invalid Git URI, must match this regex: "+`^(https?|git)://((([a-zA-Z0-9-]+\.)+[a-zA-Z]{2,})|(\d{1,3}(\.\d{1,3}){3}))(:\d+)?(/.+)\.git$`))
	}

	// Validate the ExcludedFields to ensure that it is a YAML path
	for _, fieldPath := range r.ExcludedFields {
		if !isValidYAMLPath(fieldPath) {
			errors = append(errors, field.Invalid(field.NewPath("spec").Child("excludedFields"), fieldPath, "invalid YAML path, must match this regex : "+`^([a-zA-Z0-9_./:-]*(\[[a-zA-Z0-9_*./:-]*\])?)*$`))
		}
	}

	// Validate that DefaultBranch exists if DefaultUnauthorizedUser uses a default user
	if r.DefaultUnauthorizedUserMode != syngitv1beta3.Block && r.DefaultBranch == "" {
		errors = append(errors, field.Required(field.NewPath("spec").Child("defaultBranch"), "must be set when the defaultUnauthorizedUserMode is set to UseDefaultUser"))
	}

	// Validate that no namespaces are referenced
	if r.DefaultRemoteUserRef != nil && r.DefaultRemoteUserRef.Namespace != "" {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("defaultRemoteUserRef").Child("namespace"), r.DefaultRemoteUserRef.Namespace, "should not be set as it is not supported in this version of syngit"))
	}
	if r.DefaultRemoteTargetRef != nil && r.DefaultRemoteTargetRef.Namespace != "" {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("defaultRemoteTargetRef").Child("namespace"), r.DefaultRemoteTargetRef.Namespace, "should not be set as it is not supported in this version of syngit"))
	}

	return errors
}

// isValidYAMLPath checks if the given string is a valid YAML path
func isValidYAMLPath(path string) bool {
	// Regular expression to match a valid YAML path
	yamlPathRegex := regexp.MustCompile(`^([a-zA-Z0-9_./:-]*(\[[a-zA-Z0-9_*./:-]*\])?)*$`)
	return yamlPathRegex.MatchString(path)
}

func validateRemoteSyncer(remoteSyncer *syngitv1beta3.RemoteSyncer) error {
	var allErrs field.ErrorList
	if err := validateRemoteSyncerSpec(&remoteSyncer.Spec); err != nil {
		allErrs = append(allErrs, err...)
	}

	// Validate the TargetPatterns
	rtAnnotationUserSpecific := remoteSyncer.Annotations[syngitv1beta3.RtAnnotationKeyUserSpecific]
	if !slices.Contains([]syngitv1beta3.RemoteTargetUserSpecificValues{"", syngitv1beta3.RtAnnotationValueOneUserOneBranch, syngitv1beta3.RtAnnotationValueOneUserOneBranch}, syngitv1beta3.RemoteTargetUserSpecificValues(rtAnnotationUserSpecific)) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("metadata").Child("annotations").Child(syngitv1beta3.RtAnnotationKeyUserSpecific), rtAnnotationUserSpecific,
			fmt.Sprintf("must be either %s or %s; got %s", string(syngitv1beta3.RtAnnotationValueOneUserOneBranch), string(syngitv1beta3.RtAnnotationValueOneUserOneBranch), rtAnnotationUserSpecific)))
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
	remotesyncer, ok := obj.(*syngitv1beta3.RemoteSyncer)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteSyncer object but got %T", obj)
	}
	remotesyncerlog.Info("Validation for RemoteSyncer upon creation", "name", remotesyncer.GetName())

	return nil, validateRemoteSyncer(remotesyncer)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type RemoteSyncer.
func (v *RemoteSyncerCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	remotesyncer, ok := newObj.(*syngitv1beta3.RemoteSyncer)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteSyncer object for the newObj but got %T", newObj)
	}
	remotesyncerlog.Info("Validation for RemoteSyncer upon update", "name", remotesyncer.GetName())

	return nil, validateRemoteSyncer(remotesyncer)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type RemoteSyncer.
func (v *RemoteSyncerCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	remotesyncer, ok := obj.(*syngitv1beta3.RemoteSyncer)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteSyncer object but got %T", obj)
	}
	remotesyncerlog.Info("Validation for RemoteSyncer upon deletion", "name", remotesyncer.GetName())

	return nil, nil
}
