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
	"context"
	"fmt"

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

var _ webhook.CustomValidator = &RemoteUser{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteUser) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	remoteuser, ok := obj.(*RemoteUser)
	if !ok {
		return nil, fmt.Errorf("expected a CronJob object but got %T", obj)
	}
	remoteuserlog.Info("validate update", "name", remoteuser.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteUser) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	remoteuser, ok := newObj.(*RemoteUser)
	if !ok {
		return nil, fmt.Errorf("expected a CronJob object for the newObj but got %T", newObj)
	}
	remoteuserlog.Info("validate update", "name", remoteuser.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteUser) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	remoteuser, ok := obj.(*RemoteUser)
	if !ok {
		return nil, fmt.Errorf("expected a CronJob object but got %T", obj)
	}
	remoteuserlog.Info("validate delete", "name", remoteuser.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
