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

import "sigs.k8s.io/controller-runtime/pkg/client"

// Syncer is what the syncer-scoped policies operate on: either a namespaced
// RemoteSyncer or a cluster-wide one. It exists so that a policy never reads
// .metadata.namespace directly, which is the one thing the two kinds do not
// agree on.
//
// +kubebuilder:object:generate=false
type Syncer interface {
	client.Object

	// SyncerSpec is the RemoteSyncer spec shared by both kinds.
	SyncerSpec() *RemoteSyncerSpec

	// IdentityNamespace is where this syncer's RemoteUserBindings live, and
	// where its policy-managed RemoteTargets are created.
	IdentityNamespace() string
}

var (
	_ Syncer = &RemoteSyncer{}
	_ Syncer = &ClusterWideRemoteSyncer{}
)

func (r *RemoteSyncer) SyncerSpec() *RemoteSyncerSpec { return &r.Spec }

// A namespaced RemoteSyncer keeps its bindings and targets in its own namespace.
func (r *RemoteSyncer) IdentityNamespace() string { return r.Namespace }

func (r *ClusterWideRemoteSyncer) SyncerSpec() *RemoteSyncerSpec { return &r.Spec.RemoteSyncerSpec }

// A cluster-scoped syncer has no namespace of its own, so it names one.
func (r *ClusterWideRemoteSyncer) IdentityNamespace() string {
	return r.Spec.IdentityStoreNamespace
}
