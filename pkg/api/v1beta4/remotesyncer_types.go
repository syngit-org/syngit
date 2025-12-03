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
	admissionv1 "k8s.io/api/admissionregistration/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// RemoteSyncerSpec defines the desired state of RemoteSyncer
type RemoteSyncerSpec struct {

	// remoteRepository represents the upstream repository where the RemoteTarget(s) are based on.
	// +kubebuilder:validation:Required
	// +kubebuilder:example="https://git.example.com/my-repo.git"
	// +kubebuilder:validation:Format=uri
	RemoteRepository string `json:"remoteRepository" protobuf:"bytes,1,name=remoteRepository"`

	// defaultBranch represents the upstream default branch where the RemoteTarget(s) are based on.
	// +kubebuilder:example="main"
	// +kubebuilder:default:value="main"
	// +kubebuilder:validation:Required
	DefaultBranch string `json:"defaultBranch" protobuf:"bytes,opt,2,name=defaultBranch"`

	// scopedResources defines the resources and the operations that are intercepted.
	// +kubebuilder:default:value={}
	// +kubebuilder:validation:Required
	ScopedResources ScopedResources `json:"scopedResources" protobuf:"bytes,3,name=scopedResources,casttype=ScopedResources"`

	// strategy field specify if the applied Kubernetes object must be
	// commited and applied (CommitApply) OR only commited (CommitOnly) and blocked
	// to the Kubernetes API.
	// +kubebuilder:validation:Required
	// +kubebuilder:default:value="CommitApply"
	// +kubebuilder:validation:Enum=CommitOnly;CommitApply
	Strategy Strategy `json:"strategy" protobuf:"bytes,4,name=strategy"`

	// targetStrategy is used to ensure pushing on one or multiple targets.
	// The RemoteTargets are searched based on their upstream repo & branch and
	// their association to the RemoteUserBinding that belongs to the user.
	// "OneTarget":      Use it to ensure pushing on only one target at a time.
	//                   If multiple RemoteTargets are found, then an error
	//                   will be returned from the webhook.
	// "MultipleTarget": Use it to push on all the RemoteTargets found (it can
	//                   be only one or multiple).
	// +kubebuilder:validation:Required
	// +kubebuilder:default:value="OneTarget"
	// +kubebuilder:validation:Enum=OneTarget;MultipleTarget
	TargetStrategy TargetStrategy `json:"targetStrategy" protobuf:"bytes,5,name=targetStrategy"`

	// remoteTargetSelector is a label selector that will be used when
	// the search algorithm will try to find the RemoteTarget(s) that have
	// the same upstream repo & branch
	// +kubebuilder:validation:Optional
	RemoteTargetSelector *metav1.LabelSelector `json:"remoteTargetSelector" protobuf:"bytes,opt,6,name=remoteTargetSelector"`

	// defaultBlockAppliedMessage represents the message that the webhook will
	// output if the resource is not applied (commitProcess: CommitOnly).
	// +kubebuilder:validation:Optional
	DefaultBlockAppliedMessage string `json:"defaultBlockAppliedMessage,omitempty" protobuf:"bytes,opt,7,name=defaultBlockAppliedMessage"`

	// excludedFields is a selection of key/entry of the Kubernetes object
	// that will not be pushed on the remote git repository. They will be removed
	// from the final YAML file before pushing to the remote Git repository.
	// +kubebuilder:validation:Optional
	ExcludedFields []string `json:"excludedFields,omitempty" protobuf:"bytes,opt,8,name=excludedFields"`

	// excludedFieldsConfig is a reference to a ConfigMap. The configuration
	// will be loaded from the "excludedFields" key of the ConfigMap.
	// +kubebuilder:validation:Optional
	ExcludedFieldsConfigMapRef *corev1.ObjectReference `json:"excludedFieldsConfig,omitempty" protobuf:"bytes,opt,9,name=excludedFieldsConfig"` // Ref to a ConfigMap

	// rootPath specifies the absolute root path in the remote git repository
	// where the resources scoped by this RemoteSyncer will be pushed.
	// +kubebuilder:validation:Optional
	RootPath string `json:"rootPath,omitempty" protobuf:"bytes,opt,10,name=rootPath"`

	// remoteUserBindingSelector is a label selector that will be used when
	// the search algorithm will try to find the RemoteUserBinding that belongs
	// to the Kubernetes user.
	// +kubebuilder:validation:Optional
	RemoteUserBindingSelector *metav1.LabelSelector `json:"remoteUserBindingSelector" protobuf:"bytes,opt,11,name=remoteUserBindingSelector"`

	// bypassInterceptionSubjects field is a list of Kubernetes subjects
	// (ServiceAccount or User) that can apply the resource but must not commit them.
	// +kubebuilder:validation:Optional
	BypassInterceptionSubjects []rbacv1.Subject `json:"bypassInterceptionSubjects,omitempty" protobuf:"bytes,opt,12,name=bypassInterceptionSubjects"`

	// defaultUnauthorizedUserMode defines the behavior for an unauthorized
	// user interacting with the scoped resources.
	// Can be one of these values:
	// - "Block" blocks the action on the kubernetes object and does not interact with git
	// - "UseDefaultUser" uses the .spec.defaultRemoteUserRef RemoteUser to push to git
	// +kubebuilder:default:value="Block"
	// +kubebuilder:validation:Enum=Block;UseDefaultUser
	DefaultUnauthorizedUserMode DefaultUnauthorizedUserMode `json:"defaultUnauthorizedUserMode" protobuf:"bytes,opt,13,name=defaultUnauthorizedUserMode"`

	// defaultRemoteUserRef is a reference to a RemoteUser object.
	// If specified, the RemoteUser that references the same remote git server
	// as the RemoteSyncer (with the gitBaseDomainFQDN field)
	// will be used by default to push YAMLs on the remote git repository if
	// and ONLY IF the Kubernetes user who have applied the resource does not
	// have an associated RemoteUserBinding in the same namespace
	// AND if the defaultUnauthorizedUserMode is set to 'UseDefaultUser'.
	// The resource will be pushed to the target specified by the
	// .spec.defaultRemoteTargetRef field.
	// +kubebuilder:validation:Optional
	DefaultRemoteUserRef *corev1.ObjectReference `json:"defaultRemoteUserRef,omitempty" protobuf:"bytes,opt,14,name=defaultRemoteUserRef"` // Ref to a RemoteUser object

	// defaultRemoteTargetRef  is a reference to a RemoteTarget object.
	// It will be used in the same condition as the defaultRemoteUserRef field.
	// If the defaultRemoteUserRef field is defined, then the .spec.defaultRemoteTargetRef
	// must be defined as well.
	// +kubebuilder:validation:Optional
	DefaultRemoteTargetRef *corev1.ObjectReference `json:"defaultRemoteTargetRef,omitempty" protobuf:"bytes,opt,15,name=defaultRemoteTargetRef"` // Ref to a RemoteUser object

	// insecureSkipTlsVerify skip TLS verification when set to true
	// +kubebuilder:validation:Optional
	InsecureSkipTlsVerify bool `json:"insecureSkipTlsVerify,omitempty" protobuf:"bytes,opt,16,name=insecureSkipTlsVerify"`

	// The caBundleSecretRef is a reference to a secret of type kubernetes.io/tls that stores the
	// certificate of the remote git server stored in a Secret object.
	// +kubebuilder:validation:Optional
	CABundleSecretRef corev1.SecretReference `json:"caBundleSecretRef,omitempty" protobuf:"bytes,opt,17,name=caBundleSecretRef"`
}

type RemoteSyncerStatus struct {

	// conditions represent the current state of the RemoteUser resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// lastBypassedObjectState stores the resource, the time and
	// the user info (ServiceAccount or User) of the latest resource bypassed by a subject.
	// +optional
	LastBypassedObjectState LastBypassedObjectState `json:"lastBypassedObjectState,omitempty" protobuf:"bytes,2,rep,name=lastBypassedObjectState"`

	// lastBypassedObjectState stores the resource, the time and
	// the username of the latest intercepted resource.
	// +optional
	LastObservedObjectState LastObservedObjectState `json:"lastObservedObjectState,omitempty" protobuf:"bytes,3,rep,name=lastObservedObjectState"`

	// lastBypassedObjectState stores the resource, the time, the git username, the git repositories,
	// the paths, the commit hashes and the push details of the latest intercepted resource.
	// +optional
	LastPushedObjectState LastPushedObjectState `json:"lastPushedObjectState,omitempty" protobuf:"bytes,4,rep,name=lastPushedObjectState"`
}

// +kubebuilder:subresource:status
// +kubebuilder:resource:path=remotesyncers,shortName=rsy;rsys,categories=syngit

// +kubebuilder:printcolumn:name="Last pushed resource time",type=string,JSONPath=`.status.lastPushedObjectState.lastPushedObjectTime`,priority=0
// +kubebuilder:printcolumn:name="Last pushed resource name",type=string,JSONPath=`..status.lastPushedObjectState.lastPushedObject.name`,priority=0
// +kubebuilder:printcolumn:name="Last bypassed resource time",type=string,JSONPath=`.status.lastBypassedObjectState.lastBypassObjectTime`,priority=1
// +kubebuilder:printcolumn:name="Last bypassed resource name",type=string,JSONPath=`.status.lastPushedObjectState.lastBypassObject.name`,priority=1
// +kubebuilder:printcolumn:name="Age",type=string,JSONPath=`.metadata.creationTimestamp`,priority=0

// +kubebuilder:storageversion
// +kubebuilder:object:root=true

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
	LastPushedObjectGitPaths []string `json:"lastPushedObjectGitPaths,omitempty"`

	// +optional
	LastPushedObjectGitCommitHashes []string `json:"lastPushedObjectCommitHash,omitempty"`

	// +optional
	LastPushedObject JsonGVRN `json:"lastPushedObject,omitempty"`

	// +optional
	LastPushedObjectStatus string `json:"lastPushedObjectState,omitempty"`
}
