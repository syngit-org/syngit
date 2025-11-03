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

package v1alpha3

import (
	admissionv1 "k8s.io/api/admissionregistration/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type RemoteSyncerSpec struct {
	CommitProcess CommitProcess `json:"commitProcess"`

	// +optional
	DefaultBlockAppliedMessage string `json:"defaultBlockAppliedMessage"`

	// +kubebuilder:validation:Format=uri
	RemoteRepository string `json:"remoteRepository"`

	Branch string `json:"branch"`

	// +optional
	BypassInterceptionSubjects []rbacv1.Subject `json:"bypassInterceptionSubjects,omitempty"`

	DefaultUnauthorizedUserMode DefaultUnauthorizedUserMode `json:"defaultUnauthorizedUserMode"`

	// +optional
	DefaultUser *corev1.ObjectReference `json:"defaultUser,omitempty"` // Ref to a RemoteUser object

	// +optional
	ScopedResources ScopedResources `json:"scopedResources,omitempty"`

	// +optional
	RootPath string `json:"rootPath,omitempty"`

	// +optional
	ExcludedFields []string `json:"excludedFields,omitempty"`

	// +optional
	ExcludedFieldsConfig *corev1.ObjectReference `json:"excludedFieldsConfig,omitempty"` // Ref to a ConfigMap
}

type RemoteSyncerStatus struct {

	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// +optional
	LastBypassedObjectState LastBypassedObjectState `json:"lastBypassedObjectState,omitempty"`

	// +optional
	LastObservedObjectState LastObservedObjectState `json:"lastObservedObjectState,omitempty"`

	// +optional
	LastPushedObjectState LastPushedObjectState `json:"lastPushedObjectState,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:unservedversion
// +kubebuilder:skipversion

// RemoteSyncer is the Schema for the remotesyncers API
type RemoteSyncer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteSyncerSpec   `json:"spec,omitempty"`
	Status RemoteSyncerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemoteSyncerList contains a list of RemoteSyncer
type RemoteSyncerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteSyncer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteSyncer{}, &RemoteSyncerList{})
}

/*
	SPEC EXTENSION
*/

type CommitMode string

const (
	Commit       CommitMode = "Commit"
	MergeRequest CommitMode = "MergeRequest"
)

type CommitProcess string

const (
	CommitOnly  CommitProcess = "CommitOnly"
	CommitApply CommitProcess = "CommitApply"
)

type DefaultUnauthorizedUserMode string

const (
	Block          DefaultUnauthorizedUserMode = "Block"
	UseDefaultUser DefaultUnauthorizedUserMode = "UseDefaultUser"
)

type ScopedResources struct {

	// +optional
	MatchPolicy *admissionv1.MatchPolicyType `json:"matchPolicy,omitempty" protobuf:"bytes,9,opt,name=matchPolicy,casttype=MatchPolicyType"`

	// +optional
	ObjectSelector *metav1.LabelSelector `json:"objectSelector,omitempty" protobuf:"bytes,10,opt,name=objectSelector"`

	Rules []admissionv1.RuleWithOperations `json:"rules,omitempty" protobuf:"bytes,3,rep,name=rules"`
}

type NamespaceScopedResources struct {
	APIGroups   []string `json:"apiGroups"`
	APIVersions []string `json:"apiVersions"`
	Resources   []string `json:"resources"`
	// +optional
	Names []string `json:"names"`
}

type NamespaceScopedKinds struct {
	APIGroups   []string `json:"apiGroups"`
	APIVersions []string `json:"apiVersions"`
	Kinds       []string `json:"kinds"`
	// +optional
	Names []string `json:"names"`
}

/*
	SPEC CONVERSION EXTENSION
*/

type GroupVersionKindName struct {
	*schema.GroupVersionKind
	Name string
}

type GroupVersionResourceName struct {
	*schema.GroupVersionResource
	Name string
}

/*
STATUS EXTENSION
*/

type JsonGVRN struct {
	Group    string `json:"group"`
	Version  string `json:"version"`
	Resource string `json:"resource"`
	Name     string `json:"name"`
}

type LastBypassedObjectState struct {
	// +optional
	LastBypassedObjectTime metav1.Time `json:"lastBypassObjectTime,omitempty"`

	// +optional
	LastBypassedObjectUserInfo authenticationv1.UserInfo `json:"lastBypassObjectUserInfo,omitempty"`

	// +optional
	LastBypassedObject JsonGVRN `json:"lastBypassObject,omitempty"`
}

type LastObservedObjectState struct {
	// +optional
	LastObservedObjectTime metav1.Time `json:"lastObservedObjectTime,omitempty"`

	// +optional
	LastObservedObjectUserInfo authenticationv1.UserInfo `json:"lastObservedObjectUserInfo,omitempty"`

	// +optional
	LastObservedObject JsonGVRN `json:"lastObservedObject,omitempty"`
}

type LastPushedObjectState struct {
	// +optional
	LastPushedObjectTime metav1.Time `json:"lastPushedObjectTime,omitempty"`

	// +optional
	LastPushedGitUser string `json:"lastPushedGitUser,omitempty"`

	// +optional
	LastPushedObjectGitRepo string `json:"lastPushedObjectGitRepo,omitempty"`

	// +optional
	LastPushedObjectGitPath string `json:"lastPushedObjectGitPath,omitempty"`

	// +optional
	LastPushedObjectGitCommitHash string `json:"lastPushedObjectCommitHash,omitempty"`

	// +optional
	LastPushedObject JsonGVRN `json:"lastPushedObject,omitempty"`

	// +optional
	LastPushedObjectStatus string `json:"lastPushedObjectState,omitempty"`
}
