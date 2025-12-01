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

	// secretRef is the reference to the secret that stores the Personal Access Token to the git account.
	// The Secret must be of 'kubernetes.io/basic-auth' type.
	// +kubebuilder:validation:Required
	SecretRef corev1.SecretReference `json:"secretRef" protobuf:"bytes,1,name=secretRef"`

	// email is used to do git commit.
	// +kubebuilder:validation:Required
	Email string `json:"email" protobuf:"bytes,2,name=email"`

	// gitBaseDomainFQDN is the fully qualified domain name of the git server.
	// For example: "github.com", "gitlab.com", "my-own-git-server.io", etc...
	// +kubebuilder:validation:Required
	GitBaseDomainFQDN string `json:"gitBaseDomainFQDN" protobuf:"bytes,3,name=gitBaseDomainFQDN"`
}

type RemoteUserStatus struct {

	// conditions represent the current state of the RemoteUser resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// connexionStatus is used by the syngit providers when testing the connexion.
	// It defines the status of the connexion and the associated details.
	// It can be either: "Authenticated" or "NotAuthenticated"
	// +optional
	ConnexionStatus RemoteUserConnexionStatus `json:"connexionStatus,omitempty" protobuf:"bytes,2,rep,name=connexionStatus"`

	// gitUser is the username of the user on the git server.
	// +optional
	GitUser string `json:"gitUser,omitempty" protobuf:"bytes,3,rep,name=gitUser"`

	// secretBoundStatus represents the status of the binding with the secret containing the Personal Access Token.
	// Can be one of these values:
	// - "Secret bound" when the secret is correctly bound
	// - "Secret found" when the secret is found and being processing by the controller
	// - "Secret not found" when the secret is not found
	// - "Secret type is not set to BasicAuth" when the secret is of wrong type
	// +optional
	SecretBoundStatus SecretBoundStatus `json:"secretBoundStatus,omitempty" protobuf:"bytes,4,rep,name=secretBoundStatus"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=remoteusers,shortName=ru;rus,categories=syngit

// +kubebuilder:printcolumn:name="Email",type=string,JSONPath=`.spec.email`,priority=0
// +kubebuilder:printcolumn:name="Git Server",type=string,JSONPath=`.spec.gitBaseDomainFQDN`,priority=1
// +kubebuilder:printcolumn:name="Credential status",type=string,JSONPath=`.status.secretBoundStatus`,priority=0
// +kubebuilder:printcolumn:name="Age",type=string,JSONPath=`.metadata.creationTimestamp`,priority=0

// RemoteUser is the Schema for the remoteusers API
type RemoteUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteUserSpec   `json:"spec,omitempty"`
	Status RemoteUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

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
