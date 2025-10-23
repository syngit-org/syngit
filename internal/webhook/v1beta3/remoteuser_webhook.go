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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	syngitv1beta3 "github.com/syngit-org/syngit/pkg/api/v1beta3"
)

// nolint:unused
// log is for logging in this package.
var remoteuserlog = logf.Log.WithName("remoteuser-resource")

// SetupRemoteUserWebhookWithManager registers the webhook for RemoteUser in the manager.
func SetupRemoteUserWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&syngitv1beta3.RemoteUser{}).
		WithValidator(&RemoteUserCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-syngit-io-v1beta3-remoteuser,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remoteusers,verbs=create;update,versions=v1beta3,name=vremoteuser-v1beta3.kb.io,admissionReviewVersions=v1

type RemoteUserCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &RemoteUserCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type RemoteUser.
func (v *RemoteUserCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	remoteuser, ok := obj.(*syngitv1beta3.RemoteUser)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteUser object but got %T", obj)
	}
	remoteuserlog.Info("Validation for RemoteUser upon creation", "name", remoteuser.GetName())

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type RemoteUser.
func (v *RemoteUserCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	remoteuser, ok := newObj.(*syngitv1beta3.RemoteUser)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteUser object for the newObj but got %T", newObj)
	}
	remoteuserlog.Info("Validation for RemoteUser upon update", "name", remoteuser.GetName())

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type RemoteUser.
func (v *RemoteUserCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	remoteuser, ok := obj.(*syngitv1beta3.RemoteUser)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteUser object but got %T", obj)
	}
	remoteuserlog.Info("Validation for RemoteUser upon deletion", "name", remoteuser.GetName())

	return nil, nil
}
