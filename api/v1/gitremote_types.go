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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GitRemoteSpec defines the desired state of GitRemote
type GitRemoteSpec struct {
	SecretRef corev1.SecretReference `json:"secretRef"`

	Email string `json:"email"`

	GitBaseDomainFQDN string `json:"gitBaseDomainFQDN"`

	// +optional
	CustomGitServerConfigRef corev1.ObjectReference `json:"customGitServerConfigRef,omitempty"`

	// +optional
	TestAuthentication bool `json:"testAuthentication,omitempty"`

	// +optional
	InsecureSkipTlsVerify bool `json:"insecureSkipTlsVerify,omitempty"`
}

type GitServerConfiguration struct {
	// +optional
	Inherited bool `json:"inherited,omitempty" yaml:"inherited,omitempty"`
	//+ optional
	AuthenticationEndpoint string `json:"authenticationEndpoint,omitempty" yaml:"authenticationEndpoint,omitempty"`
	// +optional
	CaBundle string `json:"caBundle,omitempty" yaml:"caBundle,omitempty"`
	// +optional
	InsecureSkipTlsVerify bool `json:"insecureSkipTlsVerify,omitempty" yaml:"insecureSkipTlsVerify,omitempty"`
}

type GitRemoteConnexionStatus struct {
	Status GitRemoteConnexionStatusReason `json:"status,omitempty"`
	// +optional
	Details string `json:"details,omitempty"`
}

type GitRemoteConnexionStatusReason string

const (
	GitConnected        GitRemoteConnexionStatusReason = "Connected"
	GitUnauthorized     GitRemoteConnexionStatusReason = "Unauthorized: bad credentials"
	GitForbidden        GitRemoteConnexionStatusReason = "Forbidden : Not enough permission"
	GitNotFound         GitRemoteConnexionStatusReason = "Not found: the git server is not found"
	GitServerError      GitRemoteConnexionStatusReason = "Server error: a server error happened"
	GitUnexpectedStatus GitRemoteConnexionStatusReason = "Unexpected response status code"
	GitNotConnected     GitRemoteConnexionStatusReason = "Not Connected"
	GitUnsupported      GitRemoteConnexionStatusReason = "Unsupported Git provider"
	GitConfigNotFound   GitRemoteConnexionStatusReason = "Git provider ConfigMap not found"
	GitConfigParseError GitRemoteConnexionStatusReason = "Failed to parse the git provider ConfigMap"
)

type SecretBoundStatus string

const (
	SecretBound     SecretBoundStatus = "Secret bound"
	SecretNotFound  SecretBoundStatus = "Secret not found"
	SecretWrongType SecretBoundStatus = "Secret type is not set to BasicAuth"
)

// GitRemoteStatus defines the observed state of GitRemote
type GitRemoteStatus struct {

	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// +optional
	ConnexionStatus GitRemoteConnexionStatus `json:"connexionStatus,omitempty"`

	// +optional
	GitUser string `json:"gitUser,omitempty"`

	// +optional
	LastAuthTime metav1.Time `json:"lastAuthTime,omitempty"`

	// +optional
	SecretBoundStatus SecretBoundStatus `json:"secretBoundStatus,omitempty"`

	// +optional
	GitServerConfiguration GitServerConfiguration `json:"gitServerConfiguration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GitRemote is the Schema for the gitremotes API
type GitRemote struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitRemoteSpec   `json:"spec,omitempty"`
	Status GitRemoteStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GitRemoteList contains a list of GitRemote
type GitRemoteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitRemote `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitRemote{}, &GitRemoteList{})
}
