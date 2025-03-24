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
	v1beta3 "github.com/syngit-org/syngit/pkg/api/v1beta3"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *RemoteSyncer) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.RemoteSyncer)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.RemoteRepository = src.Spec.RemoteRepository
	dst.Spec.DefaultBranch = src.Spec.DefaultBranch
	dst.Spec.ScopedResources = v1beta3.ScopedResources(src.Spec.ScopedResources)
	dst.Spec.Strategy = v1beta3.Strategy(src.Spec.Strategy)
	dst.Spec.TargetStrategy = v1beta3.TargetStrategy(src.Spec.TargetStrategy)
	dst.Spec.RemoteTargetSelector = src.Spec.RemoteTargetSelector
	dst.Spec.DefaultBlockAppliedMessage = src.Spec.DefaultBlockAppliedMessage
	dst.Spec.ExcludedFields = src.Spec.ExcludedFields
	dst.Spec.ExcludedFieldsConfigMapRef = src.Spec.ExcludedFieldsConfigMapRef
	dst.Spec.RootPath = src.Spec.RootPath
	dst.Spec.RemoteUserBindingSelector = src.Spec.RemoteUserBindingSelector
	dst.Spec.BypassInterceptionSubjects = src.Spec.BypassInterceptionSubjects
	dst.Spec.DefaultUnauthorizedUserMode = v1beta3.DefaultUnauthorizedUserMode(src.Spec.DefaultUnauthorizedUserMode)
	dst.Spec.DefaultRemoteUserRef = src.Spec.DefaultRemoteUserRef
	dst.Spec.DefaultRemoteTargetRef = src.Spec.DefaultRemoteTargetRef
	dst.Spec.InsecureSkipTlsVerify = src.Spec.InsecureSkipTlsVerify
	dst.Spec.CABundleSecretRef = src.Spec.CABundleSecretRef

	return nil
}

func (dst *RemoteSyncer) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.RemoteSyncer)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.RemoteRepository = src.Spec.RemoteRepository
	dst.Spec.DefaultBranch = src.Spec.DefaultBranch
	dst.Spec.ScopedResources = ScopedResources(src.Spec.ScopedResources)
	dst.Spec.Strategy = Strategy(src.Spec.Strategy)
	dst.Spec.TargetStrategy = TargetStrategy(src.Spec.TargetStrategy)
	dst.Spec.RemoteTargetSelector = src.Spec.RemoteTargetSelector
	dst.Spec.DefaultBlockAppliedMessage = src.Spec.DefaultBlockAppliedMessage
	dst.Spec.ExcludedFields = src.Spec.ExcludedFields
	dst.Spec.ExcludedFieldsConfigMapRef = src.Spec.ExcludedFieldsConfigMapRef
	dst.Spec.RootPath = src.Spec.RootPath
	dst.Spec.RemoteUserBindingSelector = src.Spec.RemoteUserBindingSelector
	dst.Spec.BypassInterceptionSubjects = src.Spec.BypassInterceptionSubjects
	dst.Spec.DefaultUnauthorizedUserMode = DefaultUnauthorizedUserMode(src.Spec.DefaultUnauthorizedUserMode)
	dst.Spec.DefaultRemoteUserRef = src.Spec.DefaultRemoteUserRef
	dst.Spec.DefaultRemoteTargetRef = src.Spec.DefaultRemoteTargetRef
	dst.Spec.InsecureSkipTlsVerify = src.Spec.InsecureSkipTlsVerify
	dst.Spec.CABundleSecretRef = src.Spec.CABundleSecretRef

	return nil
}
