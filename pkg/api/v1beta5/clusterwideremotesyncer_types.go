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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ClusterWideRemoteSyncerSpec defines the desired state of ClusterWideRemoteSyncer.
// It is a RemoteSyncerSpec plus the two inputs that a namespaced RemoteSyncer
// takes from its own namespace: which namespaces to intercept, and where the
// identities live.
type ClusterWideRemoteSyncerSpec struct {

	// Every RemoteSyncer field applies unchanged. The only difference is that
	// every reference must carry an explicit namespace, since a cluster-scoped
	// object has none of its own to default to.
	RemoteSyncerSpec `json:",inline"`

	// namespaceSelector selects the namespaces whose resources are intercepted.
	// An empty or unset selector matches every namespace.
	// Cluster-scoped resources matching .spec.scopedResources.rules are always
	// intercepted, whatever this selector says.
	// +kubebuilder:validation:Optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty" protobuf:"bytes,opt,23,name=namespaceSelector"`

	// identityStoreNamespace is the namespace holding the RemoteUserBindings that
	// map Kubernetes users to their git identity for this syncer. It is also where
	// the policy-managed RemoteTargets are created.
	// Creating a ClusterWideRemoteSyncer requires the user to be allowed to list
	// the RemoteUserBindings of that namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	IdentityStoreNamespace string `json:"identityStoreNamespace" protobuf:"bytes,24,name=identityStoreNamespace"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Last pushed resource time",type=string,JSONPath=`.status.lastPushedObjectState.lastPushedObjectTime`,priority=0
// +kubebuilder:printcolumn:name="Last pushed resource name",type=string,JSONPath=`.status.lastPushedObjectState.lastPushedObject.name`,priority=0
// +kubebuilder:printcolumn:name="Last bypassed resource time",type=string,JSONPath=`.status.lastBypassedObjectState.lastBypassObjectTime`,priority=1
// +kubebuilder:printcolumn:name="Last bypassed resource name",type=string,JSONPath=`.status.lastPushedObjectState.lastBypassObject.name`,priority=1
// +kubebuilder:printcolumn:name="Age",type=string,JSONPath=`.metadata.creationTimestamp`,priority=0
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clusterwideremotesyncers,scope=Cluster,shortName=cwrsy;cwrsys,categories=syngit
// +kubebuilder:storageversion

// ClusterWideRemoteSyncer is the Schema for the clusterwideremotesyncers API.
type ClusterWideRemoteSyncer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterWideRemoteSyncerSpec `json:"spec,omitempty"`
	Status RemoteSyncerStatus          `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterWideRemoteSyncerList contains a list of ClusterWideRemoteSyncer.
type ClusterWideRemoteSyncerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterWideRemoteSyncer `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &ClusterWideRemoteSyncer{}, &ClusterWideRemoteSyncerList{})
		return nil
	})
}
