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

package v1beta2

import (
	v1beta3 "github.com/syngit-org/syngit/pkg/api/v1beta3"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *RemoteUser) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.RemoteUser)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Email = src.Spec.Email
	gitBaseDomainFQDN := src.Spec.GitBaseDomainFQDN
	dst.Spec.GitBaseDomainFQDN = gitBaseDomainFQDN
	dst.Spec.SecretRef = src.Spec.SecretRef

	dst.Status.Conditions = src.Status.Conditions
	dst.Status.ConnexionStatus.Details = src.Status.ConnexionStatus.Details
	dst.Status.ConnexionStatus.Status = v1beta3.RemoteUserConnexionStatusReason(src.Status.ConnexionStatus.Status)
	dst.Status.GitUser = src.Status.GitUser
	dst.Status.LastAuthTime = src.Status.LastAuthTime
	dst.Status.SecretBoundStatus = v1beta3.SecretBoundStatus(src.Status.SecretBoundStatus)

	return nil
}

func (dst *RemoteUser) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.RemoteUser)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Email = src.Spec.Email
	gitBaseDomainFQDN := src.Spec.GitBaseDomainFQDN
	dst.Spec.GitBaseDomainFQDN = gitBaseDomainFQDN
	dst.Spec.SecretRef = src.Spec.SecretRef

	dst.Status.Conditions = src.Status.Conditions
	dst.Status.ConnexionStatus.Details = src.Status.ConnexionStatus.Details
	dst.Status.ConnexionStatus.Status = RemoteUserConnexionStatusReason(src.Status.ConnexionStatus.Status)
	dst.Status.GitUser = src.Status.GitUser
	dst.Status.LastAuthTime = src.Status.LastAuthTime
	dst.Status.SecretBoundStatus = SecretBoundStatus(src.Status.SecretBoundStatus)

	return nil
}
