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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RtManagedDefaultForkNamePrefix = "fork"

	RtAnnotationKeyUserSpecific      = "syngit.io/remotetarget.pattern.user-specific"
	RtAnnotationKeyOneOrManyBranches = "syngit.io/remotetarget.pattern.one-or-many-branches"

	RtLabelKeyAllowInjection = "syngit.io/remotetarget.allow-injection"
	RtLabelKeyBranch         = "syngit.io/remotetarget.branch"
	RtLabelKeyPattern        = "syngit.io/remotetarget.pattern"

	RtLabelValueOneOrManyBranches = "one-or-many-branches"
	RtLabelValueOneUserOneBranch  = "user-specific.one-user-one-branch"
)

type RemoteTargetUserSpecificValues string

const (
	RtAnnotationValueOneUserOneBranch RemoteTargetUserSpecificValues = "one-user-one-branch"
	RtAnnotationValueOneUserOneFork   RemoteTargetUserSpecificValues = "one-user-one-fork"
)

// RemoteTargetSpec defines the desired state of RemoteTarget.
type RemoteTargetSpec struct {

	// +kubebuilder:validation:Required
	// +kubebuilder:example="https://git.example.com/my-upstream-repo.git"
	// +kubebuilder:validation:Format=uri
	UpstreamRepository string `json:"upstreamRepository" protobuf:"bytes,1,name=upstreamRepository"`

	// +kubebuilder:validation:Required
	// +kubebuilder:example:"main"
	UpstreamBranch string `json:"upstreamBranch" protobuf:"bytes,2,name=upstreamBranch"`

	// +kubebuilder:validation:Required
	// +kubebuilder:example="https://git.example.com/my-target-repo.git"
	// +kubebuilder:validation:Format=uri
	TargetRepository string `json:"targetRepository" protobuf:"bytes,3,name=targetRepository"`

	// +kubebuilder:validation:Required
	// +kubebuilder:example:"main"
	TargetBranch string `json:"targetBranch" protobuf:"bytes,4,name=targetBranch"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=TryFastForwardOrDie;TryFastForwardOrHardReset;TryHardResetOrDie;""
	MergeStrategy MergeStrategy `json:"mergeStrategy" protobuf:"bytes,5,name=mergeStrategy"`
}

// RemoteTargetStatus defines the observed state of RemoteTarget.
type RemoteTargetStatus struct {

	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// +optional
	LastObservedCommitHash string `json:"lastObservedCommitHash,omitempty" protobuf:"bytes,2,rep,name=lastObservedCommitHash"`

	// +optional
	LastConsistencyOperationType string `json:"lastConsistencyOperationType,omitempty" protobuf:"bytes,3,rep,name=lastConsistencyOperationType"`

	// +optional
	LastConsistencyOperationTime string `json:"lastConsistencyOperationTime,omitempty" protobuf:"bytes,4,rep,name=lastConsistencyOperationTime"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=remotetargets,shortName=rt;rts,categories=syngit

// RemoteTarget is the Schema for the remotetargets API.
type RemoteTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteTargetSpec   `json:"spec,omitempty"`
	Status RemoteTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemoteTargetList contains a list of RemoteTarget.
type RemoteTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteTarget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteTarget{}, &RemoteTargetList{})
}

/*
	SPEC EXTENSION
*/

type MergeStrategy string

const (
	TryFastForwardOrDie       MergeStrategy = "TryFastForwardOrDie"
	TryFastForwardOrHardReset MergeStrategy = "TryFastForwardOrHardReset"
	TryHardResetOrDie         MergeStrategy = "TryHardResetOrDie"
)
