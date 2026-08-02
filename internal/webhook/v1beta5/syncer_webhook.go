/*
Copyright 2026.

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

package v1beta5

import (
	"fmt"
	"regexp"
	"slices"

	"k8s.io/apimachinery/pkg/util/validation/field"

	syngitv1beta5 "github.com/syngit-org/syngit/pkg/api/v1beta5"
)

// validateSyncer is the validation shared by RemoteSyncer and
// ClusterWideRemoteSyncer: everything that depends only on the
// RemoteSyncerSpec they both carry, and on the target-policy annotations.
// Whatever is specific to a kind is added by that kind's own validator.
func validateSyncer(syncer syngitv1beta5.Syncer) field.ErrorList {
	allErrs := validateRemoteSyncerSpec(syncer.SyncerSpec())

	// Validate the TargetPolicies
	rtAnnotationUserSpecific := syncer.GetAnnotations()[syngitv1beta5.RtAnnotationKeyUserSpecific]
	if !slices.Contains([]syngitv1beta5.RemoteTargetUserSpecificValues{"", syngitv1beta5.RtAnnotationValueOneUserOneBranch, syngitv1beta5.RtAnnotationValueOneUserOneFork}, syngitv1beta5.RemoteTargetUserSpecificValues(rtAnnotationUserSpecific)) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("metadata").Child("annotations").Child(syngitv1beta5.RtAnnotationKeyUserSpecific), rtAnnotationUserSpecific,
			fmt.Sprintf("must be either %s or %s; got %s", string(syngitv1beta5.RtAnnotationValueOneUserOneBranch), string(syngitv1beta5.RtAnnotationValueOneUserOneFork), rtAnnotationUserSpecific)))
	}

	return allErrs
}

// Validate validates the RemoteSyncerSpec
func validateRemoteSyncerSpec(r *syngitv1beta5.RemoteSyncerSpec) field.ErrorList {
	var errors field.ErrorList

	// Validate DefaultRemoteUserRef based on DefaultUnauthorizedUserMode
	if r.DefaultUnauthorizedUserMode == syngitv1beta5.BlockDefaultUser && r.DefaultRemoteUserRef != nil {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("defaultRemoteUserRef"), r.DefaultRemoteUserRef, "should not be set when defaultUnauthorizedUserMode is set to \"Block\""))
	} else if r.DefaultUnauthorizedUserMode == syngitv1beta5.UseDefaultUser && r.DefaultRemoteUserRef == nil {
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
	if r.DefaultBlockAppliedMessage != "" && r.Strategy != syngitv1beta5.CommitOnly {
		errors = append(errors, field.Forbidden(field.NewPath("spec").Child("defaultBlockAppliedMessage"), fmt.Sprintf("should not be set if strategy is not set to \"%s\"", syngitv1beta5.CommitOnly)))
	}

	// Validate that Strategy is either CommitApply or CommitOnly
	if r.Strategy != syngitv1beta5.CommitOnly && r.Strategy != syngitv1beta5.CommitApply {
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("strategy"), r.Strategy, fmt.Sprintf("must be set to \"%s\" or \"%s\"", syngitv1beta5.CommitApply, syngitv1beta5.CommitOnly)))
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

	// Validate that DefaultBranch exists if DefaultUnauthorizedUser uses a default user
	if r.DefaultUnauthorizedUserMode != syngitv1beta5.BlockDefaultUser && r.DefaultBranch == "" {
		errors = append(errors, field.Required(field.NewPath("spec").Child("defaultBranch"), "must be set when the defaultUnauthorizedUserMode is set to UseDefaultUser"))
	}

	// Referencing another namespace is allowed, but the user must be allowed to get
	// the referenced object. This is enforced by the syncer rules permissions webhooks.

	return errors
}

// isValidYAMLPath checks if the given string is a valid YAML path
func isValidYAMLPath(path string) bool {
	// Regular expression to match a valid YAML path
	yamlPathRegex := regexp.MustCompile(`^([a-zA-Z0-9_./:-]*(\[[a-zA-Z0-9_*./:-]*\])?)*$`)
	return yamlPathRegex.MatchString(path)
}
