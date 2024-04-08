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

	GitBaseDomainFQDN string `json:"gitBaseDomainFQDN"`

	// +optional
	TestAuthentication bool `json:"testAuthentication,omitempty"`

	// +optional
	GitProvider string `json:"gitProvider,omitempty"`

	// +optional
	CustomGitProvider GitProvider `json:"customGitProvider,omitempty"`
}

type GitProvider struct {
	FQDN           string `json:"fqdn"`
	Authentication string `json:"authentication"`
}

type GitRemoteConnexionStatus string

const (
	Connected        GitRemoteConnexionStatus = "Connected"
	Unauthorized     GitRemoteConnexionStatus = "Unauthorized: bad credentials"
	Forbidden        GitRemoteConnexionStatus = "Forbidden : Not enough permission"
	NotFound         GitRemoteConnexionStatus = "Not found: the git repository is not found"
	ServerError      GitRemoteConnexionStatus = "Server error: a server error happened"
	UnexpectedStatus GitRemoteConnexionStatus = "Unexpected response status code"
	Disconnected     GitRemoteConnexionStatus = "Disconnected: The secret has been deleted"
)

// GitRemoteStatus defines the observed state of GitRemote
type GitRemoteStatus struct {
	// +optional
	ConnexionStatus GitRemoteConnexionStatus `json:"connexionStatus,omitempty"`

	// +optional
	GitUserID string `json:"gitUserID,omitempty"`

	// +optional
	LastAuthTime metav1.Time `json:"lastAuthTime,omitempty"`
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
