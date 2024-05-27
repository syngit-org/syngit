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
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gertd/go-pluralize"
)

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

type GroupVersionKindName struct {
	*schema.GroupVersionKind
	Name string
}

type GroupVersionResourceName struct {
	*schema.GroupVersionResource
	Name string
}

func (nsk *NamespaceScopedKinds) NskToNsr() NamespaceScopedResources {
	nsr := NamespaceScopedResources{
		APIGroups:   nsk.APIGroups,
		APIVersions: nsk.APIVersions,
		Names:       nsk.Names,
	}
	p := pluralize.NewClient()
	for _, kind := range nsk.Kinds {
		lowercase := strings.ToLower(kind)
		nsr.Resources = append(nsr.Resources, p.Plural(lowercase))
	}

	return nsr
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

// ResourcesInterceptorSpec defines the desired state of ResourcesInterceptor
type ResourcesInterceptorSpec struct {
	CommitMode CommitMode `json:"commitMode"`

	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=3
	Operations []admissionv1.Operation `json:"operations"`

	CommitProcess CommitProcess `json:"commitProcess"`

	// +optional
	DefaultBlockAppliedMessage string `json:"defaultBlockAppliedMessage"`

	// +kubebuilder:validation:Format=uri
	RemoteRepository string `json:"remoteRepository"`

	Branch string `json:"branch"`

	// +kubebuilder:validation:MinItems=1
	AuthorizedUsers []corev1.ObjectReference `json:"authorizedUsers"` // Ref to a list of GitUserBinding object

	// +optional
	BypassInterceptionSubjects []rbacv1.Subject `json:"bypassInterceptionSubjects,omitempty"`

	DefaultUnauthorizedUserMode DefaultUnauthorizedUserMode `json:"defaultUnauthorizedUserMode"`

	// +optional
	DefaultUserBind *corev1.ObjectReference `json:"defaultUserBind,omitempty"` // Ref to a GitUserBinding object

	// +optional
	IncludedResources []NamespaceScopedResourcesPath `json:"includedResources,omitempty"`

	// +optional
	ExcludedResources []NamespaceScopedResources `json:"excludedResources,omitempty"`

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

func ParsegvknList(gvknGivenList []NamespaceScopedKinds) []GroupVersionKindName {
	gvknSet := make(map[GroupVersionKindName]bool)
	names := make([]string, 0)
	var gvknList []GroupVersionKindName

	for _, gvknGiven := range gvknGivenList {
		if len(gvknGiven.Names) != 0 {
			names = make([]string, 0)
			names = append(names, gvknGiven.Names...)
		}
		for _, group := range gvknGiven.APIGroups {
			for _, version := range gvknGiven.APIVersions {
				for _, kind := range gvknGiven.Kinds {
					if len(names) != 0 {
						for _, name := range names {
							gvkn := GroupVersionKindName{
								GroupVersionKind: &schema.GroupVersionKind{
									Group:   group,
									Version: version,
									Kind:    kind,
								},
								Name: name,
							}
							gvknSet[gvkn] = true
						}
					} else {
						gvk := GroupVersionKindName{
							GroupVersionKind: &schema.GroupVersionKind{
								Group:   group,
								Version: version,
								Kind:    kind,
							},
						}
						gvknSet[gvk] = true
					}
				}
			}
		}
	}

	for gvkn := range gvknSet {
		gvknList = append(gvknList, gvkn)
	}

	return gvknList
}

func NSKstoNSRs(nsks []NamespaceScopedKinds) []NamespaceScopedResources {

	nsrs := []NamespaceScopedResources{}
	// Transform kind into resource
	for _, nsk := range nsks {
		nsrs = append(nsrs, nsk.NskToNsr())
	}
	return nsrs
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
