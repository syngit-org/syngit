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
var remotetargetlog = logf.Log.WithName("remotetarget-resource")

// SetupRemoteTargetWebhookWithManager registers the webhook for RemoteTarget in the manager.
func SetupRemoteTargetWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&syngitv1beta3.RemoteTarget{}).
		WithValidator(&RemoteTargetCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-syngit-io-v1beta3-remotetarget,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remotetargets,verbs=create;update,versions=v1beta3,name=vremotetarget-v1beta3.kb.io,admissionReviewVersions=v1

// RemoteTargetCustomValidator struct is responsible for validating the RemoteTarget resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type RemoteTargetCustomValidator struct {
	//TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &RemoteTargetCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type RemoteTarget.
func (v *RemoteTargetCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	remotetarget, ok := obj.(*syngitv1beta3.RemoteTarget)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteTarget object but got %T", obj)
	}
	remotetargetlog.Info("Validation for RemoteTarget upon creation", "name", remotetarget.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type RemoteTarget.
func (v *RemoteTargetCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	remotetarget, ok := newObj.(*syngitv1beta3.RemoteTarget)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteTarget object for the newObj but got %T", newObj)
	}
	remotetargetlog.Info("Validation for RemoteTarget upon update", "name", remotetarget.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type RemoteTarget.
func (v *RemoteTargetCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	remotetarget, ok := obj.(*syngitv1beta3.RemoteTarget)
	if !ok {
		return nil, fmt.Errorf("expected a RemoteTarget object but got %T", obj)
	}
	remotetargetlog.Info("Validation for RemoteTarget upon deletion", "name", remotetarget.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
