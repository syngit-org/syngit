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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RemoteUserSpec struct {
	// +kubebuilder:validation:Required
	SecretRef corev1.SecretReference `json:"secretRef" protobuf:"bytes,1,name=secretRef"`

	// +kubebuilder:validation:Required
	Email string `json:"email" protobuf:"bytes,2,name=email"`

	// +kubebuilder:validation:Required
	GitBaseDomainFQDN string `json:"gitBaseDomainFQDN" protobuf:"bytes,3,name=gitBaseDomainFQDN"`
}

type RemoteUserStatus struct {

	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// +optional
	ConnexionStatus RemoteUserConnexionStatus `json:"connexionStatus,omitempty" protobuf:"bytes,2,rep,name=connexionStatus"`

	// +optional
	GitUser string `json:"gitUser,omitempty" protobuf:"bytes,3,rep,name=gitUser"`

	// +optional
	LastAuthTime metav1.Time `json:"lastAuthTime,omitempty" protobuf:"bytes,4,rep,name=lastAuthTime"`

	// +optional
	SecretBoundStatus SecretBoundStatus `json:"secretBoundStatus,omitempty" protobuf:"bytes,5,rep,name=secretBoundStatus"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion
//+kubebuilder:resource:path=remoteusers,shortName=ru;rus,categories=syngit

// RemoteUser is the Schema for the remoteusers API
type RemoteUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteUserSpec   `json:"spec,omitempty"`
	Status RemoteUserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RemoteUserList contains a list of RemoteUser
type RemoteUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteUser{}, &RemoteUserList{})
}

/*
	STATUS EXTENSION
*/

type RemoteUserConnexionStatus struct {
	Status RemoteUserConnexionStatusReason `json:"status,omitempty"`
	// +optional
	Details string `json:"details,omitempty"`
}
type RemoteUserConnexionStatusReason string

const (
	GitAuthenticated    RemoteUserConnexionStatusReason = "Authenticated"
	GitNotAuthenticated RemoteUserConnexionStatusReason = "NotAuthenticated"
)

type SecretBoundStatus string

const (
	SecretBound     SecretBoundStatus = "Secret bound"
	SecretFound     SecretBoundStatus = "Secret found"
	SecretNotFound  SecretBoundStatus = "Secret not found"
	SecretWrongType SecretBoundStatus = "Secret type is not set to BasicAuth"
)

const (
	SecretRefField = "spec.secretRef.name"
)
