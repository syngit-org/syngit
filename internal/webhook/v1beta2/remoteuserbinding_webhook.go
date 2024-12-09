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

package v1beta2

import (
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	syngitv1beta2 "syngit.io/syngit/api/v1beta2"
)

// nolint:unused
// log is for logging in this package.
var remoteuserbindinglog = logf.Log.WithName("remoteuserbinding-resource")

// SetupRemoteUserBindingWebhookWithManager registers the webhook for RemoteUserBinding in the manager.
func SetupRemoteUserBindingWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&syngitv1beta2.RemoteUserBinding{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
