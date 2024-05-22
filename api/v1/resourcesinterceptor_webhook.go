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

package v1

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
var resourcesinterceptorlog = logf.Log.WithName("resourcesinterceptor-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *ResourcesInterceptor) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-kgio-dams-kgio-v1-resourcesinterceptor,mutating=false,failurePolicy=fail,sideEffects=None,groups=kgio.dams.kgio,resources=resourcesinterceptors,verbs=create;update,versions=v1,name=vresourcesinterceptor.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &ResourcesInterceptor{}

// Validate validates the ResourcesInterceptorSpec
func (r *ResourcesInterceptorSpec) ValidateResourcesInterceptorSpec() field.ErrorList {
	var errors field.ErrorList

	// Validate DefaultUserBind based on DefaultUnauthorizedUserMode
	if r.DefaultUnauthorizedUserMode == Block && r.DefaultUserBind != nil {
		errors = append(errors, field.Invalid(field.NewPath("defaultUserBind"), r.DefaultUserBind, "should not be set when defaultUnauthorizedUserMode is set to \"Block\""))
	} else if r.DefaultUnauthorizedUserMode == UserDefaultUserBind && r.DefaultUserBind == nil {
		errors = append(errors, field.Required(field.NewPath("defaultUserBind"), "should be set when defaultUnauthorizedUserMode is set to \"UseDefaultUserBind\""))
	}

	// Validate DefaultBlockAppliedMessage only exists if CommitProcess is set to CommitApply
	if r.DefaultBlockAppliedMessage != "" && r.CommitProcess != "CommitApply" {
		errors = append(errors, field.Forbidden(field.NewPath("defaultBlockAppliedMessage"), "should not be set if .spec.commitApply is not set to \"CommitApply\""))
	}

	// For Included and Excluded Resources. Validate that if a name is specified for a resource, then the concerned resource is not referenced without the name
	errors = append(errors, r.validateFineGrainedIncludedResources(ParsegvrnList(NSRPstoNSRs(r.IncludedResources)))...)
	errors = append(errors, r.validateFineGrainedExcludedResources(ParsegvrnList(r.ExcludedResources))...)

	// Validate the ExcludedFields to ensure that it is a YAML path
	for _, fieldPath := range r.ExcludedFields {
		if !isValidYAMLPath(fieldPath) {
			errors = append(errors, field.Invalid(field.NewPath("excludedFields"), fieldPath, "must be a valid YAML path"))
		}
	}

	return errors
}

// isValidYAMLPath checks if the given string is a valid YAML path
func isValidYAMLPath(path string) bool {
	// Regular expression to match a valid YAML path
	yamlPathRegex := regexp.MustCompile(`^(\.([a-zA-Z0-9_]+|\*))+$`)
	return yamlPathRegex.MatchString(path)
}

func (r *ResourcesInterceptorSpec) searchForDuplicates(gvrns []GroupVersionResourceName) []*schema.GroupVersionResource {
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

func (r *ResourcesInterceptorSpec) validateFineGrainedIncludedResources(gvrns []GroupVersionResourceName) field.ErrorList {
	var errors field.ErrorList

	duplicates := r.searchForDuplicates(gvrns)

	if len(duplicates) > 0 {
		errors = append(errors, field.Invalid(field.NewPath("includedResources"), r.IncludedResources, "duplicate GVRName found"))
	}

	return errors
}

func (r *ResourcesInterceptorSpec) validateFineGrainedExcludedResources(gvrns []GroupVersionResourceName) field.ErrorList {
	var errors field.ErrorList

	duplicates := r.searchForDuplicates(gvrns)

	if len(duplicates) > 0 {
		errors = append(errors, field.Invalid(field.NewPath("excludedResources"), r.ExcludedResources, "duplicate GVRName found"))
	}

	return errors
}

func (r *ResourcesInterceptor) ValidateResourcesInterceptor() error {
	var allErrs field.ErrorList
	if err := r.Spec.ValidateResourcesInterceptorSpec(); err != nil {
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
func (r *ResourcesInterceptor) ValidateCreate() (admission.Warnings, error) {
	resourcesinterceptorlog.Info("validate create", "name", r.Name)

	return nil, r.ValidateResourcesInterceptor()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ResourcesInterceptor) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	resourcesinterceptorlog.Info("validate update", "name", r.Name)

	return nil, r.ValidateResourcesInterceptor()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ResourcesInterceptor) ValidateDelete() (admission.Warnings, error) {
	resourcesinterceptorlog.Info("validate delete", "name", r.Name)

	// Nothing to validate
	return nil, nil
}
