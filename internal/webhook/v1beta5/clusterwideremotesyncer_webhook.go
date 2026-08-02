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
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	syngitv1beta5 "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"github.com/syngit-org/syngit/pkg/refs"
)

// nolint:unused
// log is for logging in this package.
var clusterwideremotesyncerlog = logf.Log.WithName("clusterwideremotesyncer-resource")

// SetupClusterWideRemoteSyncerWebhookWithManager registers the webhook for ClusterWideRemoteSyncer in the manager.
func SetupClusterWideRemoteSyncerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &syngitv1beta5.ClusterWideRemoteSyncer{}).
		WithValidator(&ClusterWideRemoteSyncerCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-syngit-io-v1beta5-clusterwideremotesyncer,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=clusterwideremotesyncers,verbs=create;update,versions=v1beta5,name=vclusterwideremotesyncer-v1beta5.kb.io,admissionReviewVersions=v1

type ClusterWideRemoteSyncerCustomValidator struct {
}

// A cluster-scoped syncer is validated like a namespaced one, plus the three
// things it cannot take from a namespace it does not have.
func validateClusterWideRemoteSyncer(cwrs *syngitv1beta5.ClusterWideRemoteSyncer) error {
	allErrs := validateSyncer(cwrs)

	specPath := field.NewPath("spec")

	// A cluster-scoped object has no namespace of its own, so an unqualified
	// reference has nothing to resolve against. Reject it here rather than
	// letting every interception fail later.
	if _, err := refs.RemoteSyncerRefs(cwrs.Spec.RemoteSyncerSpec, ""); err != nil {
		allErrs = append(allErrs, field.Required(specPath, err.Error()))
	}

	if cwrs.Spec.IdentityStoreNamespace == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("identityStoreNamespace"),
			"must be set because a cluster-scoped syncer has no namespace to look for RemoteUserBindings in"))
	}

	if _, err := metav1.LabelSelectorAsSelector(cwrs.Spec.NamespaceSelector); err != nil {
		allErrs = append(allErrs, field.Invalid(specPath.Child("namespaceSelector"),
			cwrs.Spec.NamespaceSelector, err.Error()))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "syngit.io", Kind: "ClusterWideRemoteSyncer"},
		cwrs.Name, allErrs)
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ClusterWideRemoteSyncer.
func (v *ClusterWideRemoteSyncerCustomValidator) ValidateCreate(ctx context.Context, cwrs *syngitv1beta5.ClusterWideRemoteSyncer) (admission.Warnings, error) {
	clusterwideremotesyncerlog.Info("Validation for ClusterWideRemoteSyncer upon creation", "name", cwrs.GetName())

	return nil, validateClusterWideRemoteSyncer(cwrs)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ClusterWideRemoteSyncer.
func (v *ClusterWideRemoteSyncerCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newCwrs *syngitv1beta5.ClusterWideRemoteSyncer) (admission.Warnings, error) {
	clusterwideremotesyncerlog.Info("Validation for ClusterWideRemoteSyncer upon update", "name", newCwrs.GetName())

	return nil, validateClusterWideRemoteSyncer(newCwrs)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ClusterWideRemoteSyncer.
func (v *ClusterWideRemoteSyncerCustomValidator) ValidateDelete(ctx context.Context, cwrs *syngitv1beta5.ClusterWideRemoteSyncer) (admission.Warnings, error) {
	clusterwideremotesyncerlog.Info("Validation for ClusterWideRemoteSyncer upon deletion", "name", cwrs.GetName())

	return nil, nil
}
