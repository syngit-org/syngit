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
	admissionv1 "k8s.io/api/admissionregistration/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// RemoteSyncerSpec defines the desired state of RemoteSyncer
type RemoteSyncerSpec struct {

	// +kubebuilder:validation:Required
	// +kubebuilder:example="https://git.example.com/my-repo.git"
	// +kubebuilder:validation:Format=uri
	RemoteRepository string `json:"remoteRepository" protobuf:"bytes,1,name=remoteRepository"`

	// +kubebuilder:example="main"
	// +kubebuilder:default:value="main"
	// +kubebuilder:validation:Required
	DefaultBranch string `json:"defaultBranch" protobuf:"bytes,opt,2,name=defaultBranch"`

	// +kubebuilder:default:value={}
	// +kubebuilder:validation:Required
	ScopedResources ScopedResources `json:"scopedResources" protobuf:"bytes,3,name=scopedResources,casttype=ScopedResources"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default:value="CommitApply"
	// +kubebuilder:validation:Enum=CommitOnly;CommitApply
	Strategy Strategy `json:"strategy" protobuf:"bytes,4,name=strategy"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default:value="OneTarget"
	// +kubebuilder:validation:Enum=OneTarget;MultipleTarget
	TargetStrategy TargetStrategy `json:"targetStrategy" protobuf:"bytes,5,name=targetStrategy"`

	// +kubebuilder:validation:Optional
	RemoteTargetSelector *metav1.LabelSelector `json:"remoteTargetSelector" protobuf:"bytes,opt,6,name=remoteTargetSelector"`

	// +kubebuilder:validation:Optional
	DefaultBlockAppliedMessage string `json:"defaultBlockAppliedMessage,omitempty" protobuf:"bytes,opt,7,name=defaultBlockAppliedMessage"`

	// +kubebuilder:validation:Optional
	ExcludedFields []string `json:"excludedFields,omitempty" protobuf:"bytes,opt,8,name=excludedFields"`

	// +kubebuilder:validation:Optional
	ExcludedFieldsConfigMapRef *corev1.ObjectReference `json:"excludedFieldsConfig,omitempty" protobuf:"bytes,opt,9,name=excludedFieldsConfig"` // Ref to a ConfigMap

	// +kubebuilder:validation:Optional
	RootPath string `json:"rootPath,omitempty" protobuf:"bytes,opt,10,name=rootPath"`

	// +kubebuilder:validation:Optional
	RemoteUserBindingSelector *metav1.LabelSelector `json:"remoteUserBindingSelector" protobuf:"bytes,opt,11,name=remoteUserBindingSelector"`

	// +kubebuilder:validation:Optional
	BypassInterceptionSubjects []rbacv1.Subject `json:"bypassInterceptionSubjects,omitempty" protobuf:"bytes,opt,12,name=bypassInterceptionSubjects"`

	// +kubebuilder:default:value="Block"
	// +kubebuilder:validation:Enum=Block;UseDefaultUser
	DefaultUnauthorizedUserMode DefaultUnauthorizedUserMode `json:"defaultUnauthorizedUserMode" protobuf:"bytes,opt,13,name=defaultUnauthorizedUserMode"`

	// +kubebuilder:validation:Optional
	DefaultRemoteUserRef *corev1.ObjectReference `json:"defaultRemoteUserRef,omitempty" protobuf:"bytes,opt,14,name=defaultRemoteUserRef"` // Ref to a RemoteUser object

	// +kubebuilder:validation:Optional
	DefaultRemoteTargetRef *corev1.ObjectReference `json:"defaultRemoteTargetRef,omitempty" protobuf:"bytes,opt,15,name=defaultRemoteTargetRef"` // Ref to a RemoteUser object

	// +kubebuilder:validation:Optional
	InsecureSkipTlsVerify bool `json:"insecureSkipTlsVerify,omitempty" protobuf:"bytes,opt,16,name=insecureSkipTlsVerify"`

	// +kubebuilder:validation:Optional
	CABundleSecretRef corev1.SecretReference `json:"caBundleSecretRef,omitempty" protobuf:"bytes,opt,17,name=caBundleSecretRef"`
}

type RemoteSyncerStatus struct {

	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// +optional
	LastBypassedObjectState LastBypassedObjectState `json:"lastBypassedObjectState,omitempty" protobuf:"bytes,2,rep,name=lastBypassedObjectState"`

	// +optional
	LastObservedObjectState LastObservedObjectState `json:"lastObservedObjectState,omitempty" protobuf:"bytes,3,rep,name=lastObservedObjectState"`

	// +optional
	LastPushedObjectState LastPushedObjectState `json:"lastPushedObjectState,omitempty" protobuf:"bytes,4,rep,name=lastPushedObjectState"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=remotesyncers,shortName=rsy;rsys,categories=syngit

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

type TargetStrategy string

const (
	OneTarget      TargetStrategy = "OneTarget"
	MultipleTarget TargetStrategy = "MultipleTarget"
)

type TargetPattern string

const (
	OneUserOneBranch      TargetPattern = "OneUserOneBranch"
	OneOrMultipleBranches TargetPattern = "OneOrMultipleBranches"
)

type Strategy string

const (
	CommitOnly  Strategy = "CommitOnly"
	CommitApply Strategy = "CommitApply"
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
	LastObservedObjectUsername string `json:"lastObservedObjectUsername,omitempty"`

	// +optional
	LastObservedObject JsonGVRN `json:"lastObservedObject,omitempty"`
}

type LastPushedObjectState struct {
	// +optional
	LastPushedObjectTime metav1.Time `json:"lastPushedObjectTime,omitempty"`

	// +optional
	LastPushedGitUser string `json:"lastPushedGitUser,omitempty"`

	// +optional
	LastPushedObjectGitRepos []string `json:"lastPushedObjectGitRepo,omitempty"`

	// +optional
	LastPushedObjectGitPath string `json:"lastPushedObjectGitPath,omitempty"`

	// +optional
	LastPushedObjectGitCommitHashes []string `json:"lastPushedObjectCommitHash,omitempty"`

	// +optional
	LastPushedObject JsonGVRN `json:"lastPushedObject,omitempty"`

	// +optional
	LastPushedObjectStatus string `json:"lastPushedObjectState,omitempty"`
}
