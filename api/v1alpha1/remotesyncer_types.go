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

package v1alpha1

import (
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type RemoteSyncerSpec struct {
	CommitMode CommitMode `json:"commitMode"`

	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=3
	Operations []admissionregistrationv1.OperationType `json:"operations"`

	CommitProcess CommitProcess `json:"commitProcess"`

	// +optional
	DefaultBlockAppliedMessage string `json:"defaultBlockAppliedMessage"`

	// +kubebuilder:validation:Format=uri
	RemoteRepository string `json:"remoteRepository"`

	Branch string `json:"branch"`

	// +kubebuilder:validation:MinItems=1
	AuthorizedUsers []corev1.ObjectReference `json:"authorizedUsers"` // Ref to a list of RemoteUserBinding object

	// +optional
	BypassInterceptionSubjects []rbacv1.Subject `json:"bypassInterceptionSubjects,omitempty"`

	DefaultUnauthorizedUserMode DefaultUnauthorizedUserMode `json:"defaultUnauthorizedUserMode"`

	// +optional
	DefaultUserBind *corev1.ObjectReference `json:"defaultUserBind,omitempty"` // Ref to a RemoteUserBinding object

	// +optional
	IncludedResources []NamespaceScopedResourcesPath `json:"includedResources,omitempty"`

	// +optional
	ExcludedResources []NamespaceScopedResources `json:"excludedResources,omitempty"`

	// +optional
	ExcludedFields []string `json:"excludedFields,omitempty"`
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

//+kubebuilder:object:root=true
//+kubebuilder:unservedversion
//+kubebuilder:subresource:status

// RemoteSyncer is the Schema for the remotesyncers API
type RemoteSyncer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteSyncerSpec   `json:"spec,omitempty"`
	Status RemoteSyncerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

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
	Block               DefaultUnauthorizedUserMode = "Block"
	UserDefaultUserBind DefaultUnauthorizedUserMode = "UseDefaultUserBind"
)

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
	SPEC CONVERTION EXTENSION
*/

type GroupVersionKindName struct {
	*schema.GroupVersionKind
	Name string
}

type GroupVersionResourceName struct {
	*schema.GroupVersionResource
	Name string
}

func ParsegvrnList(gvrnGivenList []NamespaceScopedResources) []GroupVersionResourceName {
	gvrnSet := make(map[GroupVersionResourceName]bool)
	names := make([]string, 0)
	var gvrnList []GroupVersionResourceName

	for _, gvrnGiven := range gvrnGivenList {
		if len(gvrnGiven.Names) != 0 {
			names = make([]string, 0)
			names = append(names, gvrnGiven.Names...)
		}
		for _, group := range gvrnGiven.APIGroups {
			for _, version := range gvrnGiven.APIVersions {
				for _, resource := range gvrnGiven.Resources {
					if len(names) != 0 {
						for _, name := range names {
							gvrn := GroupVersionResourceName{
								GroupVersionResource: &schema.GroupVersionResource{
									Group:    group,
									Version:  version,
									Resource: resource,
								},
								Name: name,
							}
							gvrnSet[gvrn] = true
						}
					} else {
						gvr := GroupVersionResourceName{
							GroupVersionResource: &schema.GroupVersionResource{
								Group:    group,
								Version:  version,
								Resource: resource,
							},
						}
						gvrnSet[gvr] = true
					}
				}
			}
		}
	}

	for gvrn := range gvrnSet {
		gvrnList = append(gvrnList, gvrn)
	}

	return gvrnList
}

func GetNamesFromGVR(gvrnGivenList []NamespaceScopedResources, gvr schema.GroupVersionResource) []string {
	names := make([]string, 0)
	for _, gvrn := range ParsegvrnList(gvrnGivenList) {
		if *gvrn.GroupVersionResource == gvr && gvrn.Name != "" {
			names = append(names, gvrn.Name)
		}
	}
	return names
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

/*


	TEMP DEMO


*/

type NamespaceScopedResourcesPath struct {
	APIGroups   []string `json:"apiGroups"`
	APIVersions []string `json:"apiVersions"`
	Resources   []string `json:"resources"`
	// +optional
	Names []string `json:"names"`
	// +optional
	RepoPath string `json:"repoPath"`
}

type GroupVersionResourceNamePath struct {
	*schema.GroupVersionResource
	Name     string
	RepoPath string
}

func (nsrp *NamespaceScopedResourcesPath) nsrpToNsr() NamespaceScopedResources {
	nsr := NamespaceScopedResources{
		APIGroups:   nsrp.APIGroups,
		APIVersions: nsrp.APIVersions,
		Resources:   nsrp.Resources,
		Names:       nsrp.Names,
	}
	return nsr
}

func NSRPstoNSRs(nsrps []NamespaceScopedResourcesPath) []NamespaceScopedResources {
	nsrs := []NamespaceScopedResources{}
	for _, nsrp := range nsrps {
		nsrs = append(nsrs, nsrp.nsrpToNsr())
	}
	return nsrs
}

func GetPathFromGVRN(gvrnpGivenList []NamespaceScopedResourcesPath, gvrnGiven GroupVersionResourceName) string {
	gvrnps := parsegvrnpList(gvrnpGivenList)
	for _, gvrnp := range gvrnps {
		// if *gvrnp.GroupVersionResource == *gvrnGiven.GroupVersionResource && gvrnp.Name == gvrnGiven.Name {
		if *gvrnp.GroupVersionResource == *gvrnGiven.GroupVersionResource {
			return gvrnp.RepoPath
		}
	}
	return ""
}

func parsegvrnpList(gvrnpGivenList []NamespaceScopedResourcesPath) []GroupVersionResourceNamePath {
	gvrnpSet := make(map[GroupVersionResourceNamePath]bool)
	names := make([]string, 0)
	var gvrnpList []GroupVersionResourceNamePath

	for _, gvrnpGiven := range gvrnpGivenList {
		if len(gvrnpGiven.Names) != 0 {
			names = make([]string, 0)
			names = append(names, gvrnpGiven.Names...)
		}
		for _, group := range gvrnpGiven.APIGroups {
			for _, version := range gvrnpGiven.APIVersions {
				for _, resource := range gvrnpGiven.Resources {
					if len(names) != 0 {
						for _, name := range names {
							gvrnp := GroupVersionResourceNamePath{
								GroupVersionResource: &schema.GroupVersionResource{
									Group:    group,
									Version:  version,
									Resource: resource,
								},
								Name:     name,
								RepoPath: gvrnpGiven.RepoPath,
							}
							gvrnpSet[gvrnp] = true
						}
					} else {
						gvr := GroupVersionResourceNamePath{
							GroupVersionResource: &schema.GroupVersionResource{
								Group:    group,
								Version:  version,
								Resource: resource,
							},
							RepoPath: gvrnpGiven.RepoPath,
						}
						gvrnpSet[gvr] = true
					}
				}
			}
		}
	}

	for gvrn := range gvrnpSet {
		gvrnpList = append(gvrnpList, gvrn)
	}

	return gvrnpList
}
