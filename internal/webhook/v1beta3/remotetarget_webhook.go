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
	utils "github.com/syngit-org/syngit/pkg/utils"
)

// nolint:unused
// log is for logging in this package.
var remotetargetlog = logf.Log.WithName("remotetarget-resource")

// SetupRemoteTargetWebhookWithManager registers the webhook for RemoteTarget in the manager.
func SetupRemoteTargetWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&syngitv1beta3.RemoteTarget{}).
		WithValidator(&RemoteTargetCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-syngit-io-v1beta3-remotetarget,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remotetargets,verbs=create;update,versions=v1beta3,name=vremotetarget-v1beta3.kb.io,admissionReviewVersions=v1

type RemoteTargetCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &RemoteTargetCustomValidator{}

func validateRemoteTargetSpec(r *syngitv1beta3.RemoteTargetSpec) field.ErrorList {
	var errors field.ErrorList

	// Validate MergeStrategy
	if r.UpstreamBranch == r.TargetBranch && r.UpstreamRepository == r.TargetRepository && r.MergeStrategy != "" {
		denied := utils.SameUpstreamDifferentMergeStrategyError{
			UpstreamRepository: r.UpstreamRepository,
			UpstreamBranch:     r.UpstreamBranch,
			TargetRepository:   r.TargetRepository,
			TargetBranch:       r.TargetBranch,
			MergeStrategy:      string(r.MergeStrategy),
		}
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("mergeStrategy"), r.MergeStrategy, denied.Error()))
	}

	if (r.UpstreamBranch != r.TargetBranch || r.UpstreamRepository != r.TargetRepository) && r.MergeStrategy == "" {
		denied := utils.DifferentUpstreamEmptyMergeStrategyError{
			UpstreamRepository: r.UpstreamRepository,
			UpstreamBranch:     r.UpstreamBranch,
			TargetRepository:   r.TargetRepository,
			TargetBranch:       r.TargetBranch,
			MergeStrategy:      string(r.MergeStrategy),
		}
		errors = append(errors, field.Invalid(field.NewPath("spec").Child("mergeStrategy"), r.MergeStrategy, denied.Error()))
	}

	return errors
}

func validateRemoteTarget(remoteTarget *syngitv1beta3.RemoteTarget) error {
	var allErrs field.ErrorList
	if err := validateRemoteTargetSpec(&remoteTarget.Spec); err != nil {
		allErrs = append(allErrs, err...)
	}
	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "syngit.io", Kind: "RemoteTarget"},
		remoteTarget.Name, allErrs)
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type RemoteTarget.
func (v *RemoteTargetCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	remotetarget, ok := obj.(*syngitv1beta3.RemoteTarget)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteTarget object but got %T", obj)
	}
	remotetargetlog.Info("Validation for RemoteTarget upon creation", "name", remotetarget.GetName())

	return nil, validateRemoteTarget(remotetarget)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type RemoteTarget.
func (v *RemoteTargetCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	remotetarget, ok := newObj.(*syngitv1beta3.RemoteTarget)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteTarget object for the newObj but got %T", newObj)
	}
	remotetargetlog.Info("Validation for RemoteTarget upon update", "name", remotetarget.GetName())

	return nil, validateRemoteTarget(remotetarget)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type RemoteTarget.
func (v *RemoteTargetCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	remotetarget, ok := obj.(*syngitv1beta3.RemoteTarget)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteTarget object but got %T", obj)
	}
	remotetargetlog.Info("Validation for RemoteTarget upon deletion", "name", remotetarget.GetName())

	return nil, nil
}
