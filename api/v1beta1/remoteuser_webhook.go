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

package v1beta1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var remoteuserlog = logf.Log.WithName("remoteuser-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *RemoteUser) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-syngit-syngit-io-v1beta1-remoteuser,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.syngit.io,resources=remoteusers,verbs=create;update,versions=v1beta1,name=vremoteuser.v1beta1.syngit.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/syngit-v1beta1-remoteuser-association,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.syngit.io,resources=remoteusers,verbs=create;delete,versions=v1beta1,admissionReviewVersions=v1,name=vremoteusers-association.v1beta1.syngit.io

var _ webhook.Validator = &RemoteUser{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteUser) ValidateCreate() (admission.Warnings, error) {
	remoteuserlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteUser) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	remoteuserlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteUser) ValidateDelete() (admission.Warnings, error) {
	remoteuserlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
