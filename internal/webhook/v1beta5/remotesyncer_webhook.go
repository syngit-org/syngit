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
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	syngitv1beta5 "github.com/syngit-org/syngit/pkg/api/v1beta5"
)

// nolint:unused
// log is for logging in this package.
var remotesyncerlog = logf.Log.WithName("remotesyncer-resource")

// SetupRemoteSyncerWebhookWithManager registers the webhook for RemoteSyncer in the manager.
func SetupRemoteSyncerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &syngitv1beta5.RemoteSyncer{}).
		WithValidator(&RemoteSyncerCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-syngit-io-v1beta5-remotesyncer,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remotesyncers,verbs=create;update,versions=v1beta5,name=vremotesyncer-v1beta5.kb.io,admissionReviewVersions=v1

type RemoteSyncerCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// A namespaced RemoteSyncer adds nothing to the shared syncer validation: the
// namespace it needs to intercept, to resolve references against and to find
// identities in is its own.
func validateRemoteSyncer(remoteSyncer *syngitv1beta5.RemoteSyncer) error {
	allErrs := validateSyncer(remoteSyncer)

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "syngit.io", Kind: "RemoteSyncer"},
		remoteSyncer.Name, allErrs)
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type RemoteSyncer.
func (v *RemoteSyncerCustomValidator) ValidateCreate(ctx context.Context, remotesyncer *syngitv1beta5.RemoteSyncer) (admission.Warnings, error) {
	remotesyncerlog.Info("Validation for RemoteSyncer upon creation", "name", remotesyncer.GetName())

	return nil, validateRemoteSyncer(remotesyncer)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type RemoteSyncer.
func (v *RemoteSyncerCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newRemotesyncer *syngitv1beta5.RemoteSyncer) (admission.Warnings, error) {
	remotesyncerlog.Info("Validation for RemoteSyncer upon update", "name", newRemotesyncer.GetName())

	return nil, validateRemoteSyncer(newRemotesyncer)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type RemoteSyncer.
func (v *RemoteSyncerCustomValidator) ValidateDelete(ctx context.Context, remotesyncer *syngitv1beta5.RemoteSyncer) (admission.Warnings, error) {
	remotesyncerlog.Info("Validation for RemoteSyncer upon deletion", "name", remotesyncer.GetName())

	return nil, nil
}
