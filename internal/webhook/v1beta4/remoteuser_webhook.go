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

package v1beta4

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	syngitv1beta4 "github.com/syngit-org/syngit/pkg/api/v1beta4"
)

// nolint:unused
// log is for logging in this package.
var remoteuserlog = logf.Log.WithName("remoteuser-resource")

// SetupRemoteUserWebhookWithManager registers the webhook for RemoteUser in the manager.
func SetupRemoteUserWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &syngitv1beta4.RemoteUser{}).
		WithValidator(&RemoteUserCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-syngit-io-v1beta4-remoteuser,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remoteusers,verbs=create;update,versions=v1beta4,name=vremoteuser-v1beta4.kb.io,admissionReviewVersions=v1

type RemoteUserCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type RemoteUser.
func (v *RemoteUserCustomValidator) ValidateCreate(ctx context.Context, remoteuser *syngitv1beta4.RemoteUser) (admission.Warnings, error) {
	remoteuserlog.Info("Validation for RemoteUser upon creation", "name", remoteuser.GetName())

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type RemoteUser.
func (v *RemoteUserCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newRemoteuser *syngitv1beta4.RemoteUser) (admission.Warnings, error) {
	remoteuserlog.Info("Validation for RemoteUser upon update", "name", newRemoteuser.GetName())

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type RemoteUser.
func (v *RemoteUserCustomValidator) ValidateDelete(ctx context.Context, remoteuser *syngitv1beta4.RemoteUser) (admission.Warnings, error) {
	remoteuserlog.Info("Validation for RemoteUser upon deletion", "name", remoteuser.GetName())

	return nil, nil
}
