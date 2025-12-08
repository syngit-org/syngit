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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RtManagedNamePrefix            = "rt"
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

	// upstreamRepository is used to ensure the mapping with
	// the RemoteSyncer(s) that defines it as the default repository.
	// It will also be used for the merge strategies.
	// +kubebuilder:validation:Required
	// +kubebuilder:example="https://git.example.com/my-upstream-repo.git"
	// +kubebuilder:validation:Format=uri
	UpstreamRepository string `json:"upstreamRepository" protobuf:"bytes,1,name=upstreamRepository"`

	// upstreamBranch is used to ensure the mapping with
	// the RemoteSyncer(s) that defines it as the default repository.
	// It will also be used for the merge strategies.
	// +kubebuilder:validation:Required
	// +kubebuilder:example:"main"
	UpstreamBranch string `json:"upstreamBranch" protobuf:"bytes,2,name=upstreamBranch"`

	// targetRepository defines the repository where the
	// resource should be pushed. It can be the same as the upstream
	// repository.
	// +kubebuilder:validation:Required
	// +kubebuilder:example="https://git.example.com/my-target-repo.git"
	// +kubebuilder:validation:Format=uri
	TargetRepository string `json:"targetRepository" protobuf:"bytes,3,name=targetRepository"`

	// targetBranch defines the branch where the resource
	// should be pushed. It can be the same as the upstream branch.
	// +kubebuilder:validation:Required
	// +kubebuilder:example:"main"
	TargetBranch string `json:"targetBranch" protobuf:"bytes,4,name=targetBranch"`

	//  mergeStrategy defines the strategy that must be used to get
	// the commits from the upstream. It must be empty if the target
	// repository and branch are the same.
	// Can be one of these values:
	// - TryFastForwardOrDie:       Try to pull the changes from the upstream.
	//                              If it is not a fast forward, the webhook
	//                              will return an error instead of pushing the
	//                              resource.
	// - TryHardResetOrDie:         Try to do an hard reset on the latest commit
	//                              of the upstream.
	// - TryFastForwardOrHardReset: First try the fast forward strategy,
	//                              if there is an error, then try the hard
	//                              reset strategy.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=TryFastForwardOrDie;TryFastForwardOrHardReset;TryHardResetOrDie;""
	MergeStrategy MergeStrategy `json:"mergeStrategy" protobuf:"bytes,5,name=mergeStrategy"`
}

// RemoteTargetStatus defines the observed state of RemoteTarget.
type RemoteTargetStatus struct {

	// conditions represents the current state of the RemoteUser resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:resource:path=remotetargets,shortName=rt;rts,categories=syngit

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

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
