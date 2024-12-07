//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1beta2

import (
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GitUserHost) DeepCopyInto(out *GitUserHost) {
	*out = *in
	out.SecretRef = in.SecretRef
	in.LastUsedTime.DeepCopyInto(&out.LastUsedTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GitUserHost.
func (in *GitUserHost) DeepCopy() *GitUserHost {
	if in == nil {
		return nil
	}
	out := new(GitUserHost)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GroupVersionKindName) DeepCopyInto(out *GroupVersionKindName) {
	*out = *in
	if in.GroupVersionKind != nil {
		in, out := &in.GroupVersionKind, &out.GroupVersionKind
		*out = new(schema.GroupVersionKind)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GroupVersionKindName.
func (in *GroupVersionKindName) DeepCopy() *GroupVersionKindName {
	if in == nil {
		return nil
	}
	out := new(GroupVersionKindName)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GroupVersionResourceName) DeepCopyInto(out *GroupVersionResourceName) {
	*out = *in
	if in.GroupVersionResource != nil {
		in, out := &in.GroupVersionResource, &out.GroupVersionResource
		*out = new(schema.GroupVersionResource)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GroupVersionResourceName.
func (in *GroupVersionResourceName) DeepCopy() *GroupVersionResourceName {
	if in == nil {
		return nil
	}
	out := new(GroupVersionResourceName)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *JsonGVRN) DeepCopyInto(out *JsonGVRN) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new JsonGVRN.
func (in *JsonGVRN) DeepCopy() *JsonGVRN {
	if in == nil {
		return nil
	}
	out := new(JsonGVRN)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LastBypassedObjectState) DeepCopyInto(out *LastBypassedObjectState) {
	*out = *in
	in.LastBypassedObjectTime.DeepCopyInto(&out.LastBypassedObjectTime)
	in.LastBypassedObjectUserInfo.DeepCopyInto(&out.LastBypassedObjectUserInfo)
	out.LastBypassedObject = in.LastBypassedObject
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LastBypassedObjectState.
func (in *LastBypassedObjectState) DeepCopy() *LastBypassedObjectState {
	if in == nil {
		return nil
	}
	out := new(LastBypassedObjectState)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LastObservedObjectState) DeepCopyInto(out *LastObservedObjectState) {
	*out = *in
	in.LastObservedObjectTime.DeepCopyInto(&out.LastObservedObjectTime)
	in.LastObservedObjectUserInfo.DeepCopyInto(&out.LastObservedObjectUserInfo)
	out.LastObservedObject = in.LastObservedObject
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LastObservedObjectState.
func (in *LastObservedObjectState) DeepCopy() *LastObservedObjectState {
	if in == nil {
		return nil
	}
	out := new(LastObservedObjectState)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LastPushedObjectState) DeepCopyInto(out *LastPushedObjectState) {
	*out = *in
	in.LastPushedObjectTime.DeepCopyInto(&out.LastPushedObjectTime)
	out.LastPushedObject = in.LastPushedObject
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LastPushedObjectState.
func (in *LastPushedObjectState) DeepCopy() *LastPushedObjectState {
	if in == nil {
		return nil
	}
	out := new(LastPushedObjectState)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespaceScopedKinds) DeepCopyInto(out *NamespaceScopedKinds) {
	*out = *in
	if in.APIGroups != nil {
		in, out := &in.APIGroups, &out.APIGroups
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.APIVersions != nil {
		in, out := &in.APIVersions, &out.APIVersions
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Kinds != nil {
		in, out := &in.Kinds, &out.Kinds
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Names != nil {
		in, out := &in.Names, &out.Names
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespaceScopedKinds.
func (in *NamespaceScopedKinds) DeepCopy() *NamespaceScopedKinds {
	if in == nil {
		return nil
	}
	out := new(NamespaceScopedKinds)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespaceScopedResources) DeepCopyInto(out *NamespaceScopedResources) {
	*out = *in
	if in.APIGroups != nil {
		in, out := &in.APIGroups, &out.APIGroups
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.APIVersions != nil {
		in, out := &in.APIVersions, &out.APIVersions
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Names != nil {
		in, out := &in.Names, &out.Names
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespaceScopedResources.
func (in *NamespaceScopedResources) DeepCopy() *NamespaceScopedResources {
	if in == nil {
		return nil
	}
	out := new(NamespaceScopedResources)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteSyncer) DeepCopyInto(out *RemoteSyncer) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteSyncer.
func (in *RemoteSyncer) DeepCopy() *RemoteSyncer {
	if in == nil {
		return nil
	}
	out := new(RemoteSyncer)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RemoteSyncer) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteSyncerList) DeepCopyInto(out *RemoteSyncerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RemoteSyncer, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteSyncerList.
func (in *RemoteSyncerList) DeepCopy() *RemoteSyncerList {
	if in == nil {
		return nil
	}
	out := new(RemoteSyncerList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RemoteSyncerList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteSyncerSpec) DeepCopyInto(out *RemoteSyncerSpec) {
	*out = *in
	in.ScopedResources.DeepCopyInto(&out.ScopedResources)
	if in.ExcludedFields != nil {
		in, out := &in.ExcludedFields, &out.ExcludedFields
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ExcludedFieldsConfigMapRef != nil {
		in, out := &in.ExcludedFieldsConfigMapRef, &out.ExcludedFieldsConfigMapRef
		*out = new(v1.ObjectReference)
		**out = **in
	}
	if in.BypassInterceptionSubjects != nil {
		in, out := &in.BypassInterceptionSubjects, &out.BypassInterceptionSubjects
		*out = make([]rbacv1.Subject, len(*in))
		copy(*out, *in)
	}
	if in.DefaultRemoteUserRef != nil {
		in, out := &in.DefaultRemoteUserRef, &out.DefaultRemoteUserRef
		*out = new(v1.ObjectReference)
		**out = **in
	}
	out.CABundleSecretRef = in.CABundleSecretRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteSyncerSpec.
func (in *RemoteSyncerSpec) DeepCopy() *RemoteSyncerSpec {
	if in == nil {
		return nil
	}
	out := new(RemoteSyncerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteSyncerStatus) DeepCopyInto(out *RemoteSyncerStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.LastBypassedObjectState.DeepCopyInto(&out.LastBypassedObjectState)
	in.LastObservedObjectState.DeepCopyInto(&out.LastObservedObjectState)
	in.LastPushedObjectState.DeepCopyInto(&out.LastPushedObjectState)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteSyncerStatus.
func (in *RemoteSyncerStatus) DeepCopy() *RemoteSyncerStatus {
	if in == nil {
		return nil
	}
	out := new(RemoteSyncerStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteUser) DeepCopyInto(out *RemoteUser) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteUser.
func (in *RemoteUser) DeepCopy() *RemoteUser {
	if in == nil {
		return nil
	}
	out := new(RemoteUser)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RemoteUser) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteUserBinding) DeepCopyInto(out *RemoteUserBinding) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteUserBinding.
func (in *RemoteUserBinding) DeepCopy() *RemoteUserBinding {
	if in == nil {
		return nil
	}
	out := new(RemoteUserBinding)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RemoteUserBinding) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteUserBindingList) DeepCopyInto(out *RemoteUserBindingList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RemoteUserBinding, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteUserBindingList.
func (in *RemoteUserBindingList) DeepCopy() *RemoteUserBindingList {
	if in == nil {
		return nil
	}
	out := new(RemoteUserBindingList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RemoteUserBindingList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteUserBindingSpec) DeepCopyInto(out *RemoteUserBindingSpec) {
	*out = *in
	out.Subject = in.Subject
	if in.RemoteRefs != nil {
		in, out := &in.RemoteRefs, &out.RemoteRefs
		*out = make([]v1.ObjectReference, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteUserBindingSpec.
func (in *RemoteUserBindingSpec) DeepCopy() *RemoteUserBindingSpec {
	if in == nil {
		return nil
	}
	out := new(RemoteUserBindingSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteUserBindingStatus) DeepCopyInto(out *RemoteUserBindingStatus) {
	*out = *in
	if in.GitUserHosts != nil {
		in, out := &in.GitUserHosts, &out.GitUserHosts
		*out = make([]GitUserHost, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.LastUsedTime.DeepCopyInto(&out.LastUsedTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteUserBindingStatus.
func (in *RemoteUserBindingStatus) DeepCopy() *RemoteUserBindingStatus {
	if in == nil {
		return nil
	}
	out := new(RemoteUserBindingStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteUserConnexionStatus) DeepCopyInto(out *RemoteUserConnexionStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteUserConnexionStatus.
func (in *RemoteUserConnexionStatus) DeepCopy() *RemoteUserConnexionStatus {
	if in == nil {
		return nil
	}
	out := new(RemoteUserConnexionStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteUserList) DeepCopyInto(out *RemoteUserList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RemoteUser, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteUserList.
func (in *RemoteUserList) DeepCopy() *RemoteUserList {
	if in == nil {
		return nil
	}
	out := new(RemoteUserList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RemoteUserList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteUserSpec) DeepCopyInto(out *RemoteUserSpec) {
	*out = *in
	out.SecretRef = in.SecretRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteUserSpec.
func (in *RemoteUserSpec) DeepCopy() *RemoteUserSpec {
	if in == nil {
		return nil
	}
	out := new(RemoteUserSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteUserStatus) DeepCopyInto(out *RemoteUserStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	out.ConnexionStatus = in.ConnexionStatus
	in.LastAuthTime.DeepCopyInto(&out.LastAuthTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteUserStatus.
func (in *RemoteUserStatus) DeepCopy() *RemoteUserStatus {
	if in == nil {
		return nil
	}
	out := new(RemoteUserStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ScopedResources) DeepCopyInto(out *ScopedResources) {
	*out = *in
	if in.MatchPolicy != nil {
		in, out := &in.MatchPolicy, &out.MatchPolicy
		*out = new(admissionregistrationv1.MatchPolicyType)
		**out = **in
	}
	if in.ObjectSelector != nil {
		in, out := &in.ObjectSelector, &out.ObjectSelector
		*out = new(metav1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.Rules != nil {
		in, out := &in.Rules, &out.Rules
		*out = make([]admissionregistrationv1.RuleWithOperations, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ScopedResources.
func (in *ScopedResources) DeepCopy() *ScopedResources {
	if in == nil {
		return nil
	}
	out := new(ScopedResources)
	in.DeepCopyInto(out)
	return out
}
