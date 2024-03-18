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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GitUserBindingSpec defines the desired state of GitUserBinding
type GitUserBindingSpec struct {
	Subject   rbacv1.Subject         `json:"subject"`
	RemoteRef corev1.ObjectReference `json:"remoteRef"` // Ref to a GitRemote object
}

type GitUserBindingState string

const (
	Bound    GitUserBindingState = "Bound"
	NotBound GitUserBindingState = "Not bound"
)

// GitUserBindingStatus defines the observed state of GitUserBinding
type GitUserBindingStatus struct {
	// +optional
	State GitUserBindingState `json:"state,omitempty"`

	// +optional
	UserGitID string `json:"userGitID,omitempty"`

	// +optional
	UserKubernetesID string `json:"userKubernetesID,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GitUserBinding is the Schema for the gituserbindings API
type GitUserBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitUserBindingSpec   `json:"spec,omitempty"`
	Status GitUserBindingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GitUserBindingList contains a list of GitUserBinding
type GitUserBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitUserBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitUserBinding{}, &GitUserBindingList{})
}
