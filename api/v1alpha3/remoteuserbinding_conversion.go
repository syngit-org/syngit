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

package v1alpha3

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"syngit.io/syngit/api/v1beta1"
)

func (src *RemoteUserBinding) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.RemoteUserBinding)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.RemoteRefs = src.Spec.RemoteRefs
	dst.Spec.Subject = src.Spec.Subject

	return nil
}

func (dst *RemoteUserBinding) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.RemoteUserBinding)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.RemoteRefs = src.Spec.RemoteRefs
	dst.Spec.Subject = src.Spec.Subject

	return nil
}
