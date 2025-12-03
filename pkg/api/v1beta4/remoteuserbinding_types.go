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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RubNamePrefix           = "associated-rub"
	RubAnnotationKeyManaged = "syngit.io/remoteuserbinding.managed"
	RemoteRefsField         = "spec.remoteRefs"
)

type RemoteUserBindingSpec struct {

	// subject is the Kubernetes User/ServiceAccount that is bound to the listed RemoteUsers & RemoteTargets.
	// +kubebuilder:validation:Required
	Subject rbacv1.Subject `json:"subject" protobuf:"bytes,1,name=subject"`

	// remoteUserRefs is a list of reference to RemoteUser(s) that are bound to the subject.
	// +kubebuilder:validation:Required
	RemoteUserRefs []corev1.ObjectReference `json:"remoteUserRefs" protobuf:"bytes,2,name=remoteUserRefs"` // Ref to the listed RemoteUser objects

	// remoteTargetRefs is a list of reference to RemoteTarget(s) that are bound to the subject.
	// +kubebuilder:validation:Optional
	RemoteTargetRefs []corev1.ObjectReference `json:"remoteTargetRefs" protobuf:"bytes,3,name=remoteTargetRefs"` // Ref to the listed RemoteTarget objects
}

type RemoteUserBindingStatus struct {

	// conditions represents the current state of the RemoteUser resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// remoteUserState represents the overall state of the RemoteUser(s) secret.
	// Can be one of these values:
	// - "AllBound" when the secret of all the RemoteUser(s) are bound
	// - "PartiallyBound" when some secret are bound and some other are not
	// - "NoneBound" when no secret among all the RemoteUser are bound
	// +optional
	RemoteUserState RemoteUserBindingState `json:"remoteUserState,omitempty" protobuf:"bytes,2,rep,name=remoteUserState"`

	// remoteUserHosts tracks the spec and the status of each RemoteUser.
	// It describes the name of the RemoteUser, the secret reference, the state of the secret, the git server FQDN and the last time it has been used.
	// +optional
	RemoteUserHosts []RemoteUserHost `json:"remoteUserHosts" protobuf:"bytes,3,rep,name=remoteUserHosts"`

	// userKubernetesID is the ID of the Kubernetes user
	// +optional
	UserKubernetesID string `json:"userKubernetesID,omitempty" protobuf:"bytes,4,rep,name=userKubernetesID"`
}

// +kubebuilder:subresource:status
// +kubebuilder:resource:path=remoteuserbindings,shortName=rub;rubs,categories=syngit

// +kubebuilder:printcolumn:name="Kubernetes User ID",type=string,JSONPath=`.status.userKubernetesID`,priority=0
// +kubebuilder:printcolumn:name="Remote Users State",type=string,JSONPath=`.status.remoteUserState`,priority=1
// +kubebuilder:printcolumn:name="Age",type=string,JSONPath=`.metadata.creationTimestamp`,priority=0

// +kubebuilder:storageversion
// +kubebuilder:object:root=true

// RemoteUserBinding is the Schema for the remoteuserbindings API
type RemoteUserBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteUserBindingSpec   `json:"spec,omitempty"`
	Status RemoteUserBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemoteUserBindingList contains a list of RemoteUserBinding
type RemoteUserBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteUserBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteUserBinding{}, &RemoteUserBindingList{})
}

/*
	STATUS EXTENSION
*/

type RemoteUserBindingState string

const (
	AllBound       RemoteUserBindingState = "AllBound"
	PartiallyBound RemoteUserBindingState = "PartiallyBound"
	NoneBound      RemoteUserBindingState = "NoneBound"
	Bound          RemoteUserBindingState = "Bound"
	NotBound       RemoteUserBindingState = "NotBound"
)

type RemoteUserHost struct {
	RemoteUserUsed string                 `json:"remoteUserUsed,omitempty"`
	SecretRef      corev1.SecretReference `json:"secretRef"`
	GitFQDN        string                 `json:"gitFQDN,omitempty"`
	State          RemoteUserBindingState `json:"state,omitempty"`
}
