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
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RubNamePrefix           = "associated-rub"
	RubAnnotationKeyManaged = "syngit.io/remoteuserbinding.managed"
	RemoteRefsField         = "spec.remoteRefs"
)

type RemoteUserBindingSpec struct {
	// +kubebuilder:validation:Required
	Subject rbacv1.Subject `json:"subject" protobuf:"bytes,1,name=subject"`

	// +kubebuilder:validation:Required
	RemoteUserRefs []corev1.ObjectReference `json:"remoteUserRefs" protobuf:"bytes,2,name=remoteUserRefs"` // Ref to the listed RemoteUser objects

	// +kubebuilder:validation:Optional
	RemoteTargetRefs []corev1.ObjectReference `json:"remoteTargetRefs" protobuf:"bytes,3,name=remoteTargetRefs"` // Ref to the listed RemoteTarget objects
}

type RemoteUserBindingStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// +optional
	State GitUserBindingState `json:"state,omitempty" protobuf:"bytes,2,rep,name=state"`

	// +optional
	GitUserHosts []GitUserHost `json:"gitUserHosts" protobuf:"bytes,3,rep,name=gitUserHosts"`

	// +optional
	UserKubernetesID string `json:"userKubernetesID,omitempty" protobuf:"bytes,4,rep,name=userKubernetesID"`

	// +optional
	LastUsedTime metav1.Time `json:"lastUsedTime,omitempty" protobuf:"bytes,5,rep,name=lastUsedTime"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=remoteuserbindings,shortName=rub;rubs,categories=syngit

// RemoteUserBinding is the Schema for the remoteuserbindings API
type RemoteUserBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteUserBindingSpec   `json:"spec,omitempty"`
	Status RemoteUserBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemoteUserBindingList contains a list of RemoteUserBinding
type RemoteUserBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteUserBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteUserBinding{}, &RemoteUserBindingList{})
}

/*
	STATUS EXTENSION
*/

type GitUserBindingState string

const (
	AllBound       GitUserBindingState = "AllBound"
	PartiallyBound GitUserBindingState = "PartiallyBound"
	NoneBound      GitUserBindingState = "NoneBound"
	Bound          GitUserBindingState = "Bound"
	NotBound       GitUserBindingState = "NotBound"
)

type GitUserHost struct {
	RemoteUserUsed string                 `json:"remoteUserUsed,omitempty"`
	SecretRef      corev1.SecretReference `json:"secretRef"`
	GitFQDN        string                 `json:"gitFQDN,omitempty"`
	State          GitUserBindingState    `json:"state,omitempty"`
	LastUsedTime   metav1.Time            `json:"lastUsedTime,omitempty"`
}
