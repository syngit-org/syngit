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

type CommitMode string

const (
	Commit       CommitMode = "Commit"
	MergeRequest CommitMode = "MergeRequest"
)

type OperationType string

const (
	Create OperationType = "CREATE"
	Update OperationType = "UPDATE"
	Delete OperationType = "DELETE"
)

type CommitProcess string

const (
	CommitOnly  CommitProcess = "CommitOnly"
	ApplyCommit CommitProcess = "ApplyCommit"
)

type DefaultUnauthorizedUserMode string

const (
	Block               DefaultUnauthorizedUserMode = "Block"
	UserDefaultUserBind DefaultUnauthorizedUserMode = "UseDefaultUserBind"
)

type NamespaceScopedKinds struct {
	APIGroups   []string `json:"apiGroups"`
	APIVersions []string `json:"apiVersions"`
	Kinds       []string `json:"kinds"`
}

// ResourcesInterceptorSpec defines the desired state of ResourcesInterceptor
type ResourcesInterceptorSpec struct {
	CommitMode CommitMode `json:"commitMode"`

	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=3
	Operations []OperationType `json:"operations"`

	CommitProcess CommitProcess `json:"commitProcess"`

	// +kubebuilder:validation:Format=uri
	RemoteRepository string `json:"remoteRepository"`

	// +kubebuilder:validation:MinItems=1
	AuthorizedUsers []corev1.ObjectReference `json:"authorizedUsers"` // Ref to a list of GitUserBinding object

	// +optional
	BypassInterceptionSubjects []rbacv1.Subject `json:"bypassInterceptionSubjects,omitempty"`

	DefaultUnauthorizedUserMode DefaultUnauthorizedUserMode `json:"defaultUnauthorizedUserMode"`

	// +optional
	DefaultUserBind *corev1.ObjectReference `json:"defaultUserBind,omitempty"` // Ref to a GitUserBinding object

	// +optional
	IncludedResources []NamespaceScopedKinds `json:"includedResources,omitempty"`

	// +optional
	ExcludedResources []NamespaceScopedKinds `json:"excludedResources,omitempty"`

	// +optional
	ExcludedFields []string `json:"excludedFields,omitempty"`
}

type NamespaceScopedObject struct {
	APIGroups   metav1.APIGroup `json:"apiGroups"`
	APIVersions string          `json:"apiVersions"`
	Resources   string          `json:"resources"`
	Name        string          `json:"name"`
}

type LastBypassedObjectState struct {
	// +optional
	LastBypassedObjectTime metav1.Time `json:"lastBypassObjectTime,omitempty"`

	// +optional
	LastBypassedObjectSubject rbacv1.Subject `json:"lastBypassObjectSubject,omitempty"`

	// +optional
	LastBypassedObject NamespaceScopedObject `json:"lastBypassObject,omitempty"`
}

type LastInterceptedObjectState struct {
	// +optional
	LastInterceptedObjectTime metav1.Time `json:"lastInterceptedObjectTime,omitempty"`

	// +optional
	LastInterceptedObjectKubernetesUser rbacv1.Subject `json:"lastInterceptedObjectKubernetesUser,omitempty"`

	// +optional
	LastInterceptedObject NamespaceScopedObject `json:"lastInterceptedObject,omitempty"`
}

type LastPushedObjectState struct {
	// +optional
	LastPushedObjectTime metav1.Time `json:"lastPushedObjectTime,omitempty"`

	// +optional
	LastPushedGitUserID string `json:"lastPushedGitUserID,omitempty"`

	// +optional
	LastPushedObjectGitPath string `json:"lastPushedObjectGitPath,omitempty"`

	// +optional
	LastPushedObject NamespaceScopedObject `json:"lastPushedObject,omitempty"`

	// +optional
	LastPushedObjectStatus PushedObjectStatus `json:"lastPushedObjectState,omitempty"`
}

type PushedObjectStatus string

const (
	Pushed         PushedObjectStatus = "Resource correctly pushed"
	PushNotAllowed PushedObjectStatus = "Error: Push permission is not allowed on this git repository for this user"
	NetworkError   PushedObjectStatus = "Error: A network error occured"
)

// ResourcesInterceptorStatus defines the observed state of ResourcesInterceptor
type ResourcesInterceptorStatus struct {
	// +optional
	LastBypassedObjectState LastBypassedObjectState `json:"lastBypassedObjectState,omitempty"`

	// +optional
	LastInterceptedObjectState LastInterceptedObjectState `json:"lastInterceptedObjectState,omitempty"`

	// +optional
	LastPushedObjectState LastPushedObjectState `json:"lastPushedObjectState,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ResourcesInterceptor is the Schema for the resourcesinterceptors API
type ResourcesInterceptor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourcesInterceptorSpec   `json:"spec,omitempty"`
	Status ResourcesInterceptorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ResourcesInterceptorList contains a list of ResourcesInterceptor
type ResourcesInterceptorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourcesInterceptor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourcesInterceptor{}, &ResourcesInterceptorList{})
}
