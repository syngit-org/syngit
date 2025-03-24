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

func (src *RemoteTarget) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.RemoteTarget)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.UpstreamRepository = src.Spec.UpstreamRepository
	dst.Spec.UpstreamBranch = src.Spec.UpstreamBranch
	dst.Spec.TargetRepository = src.Spec.TargetRepository
	dst.Spec.TargetBranch = src.Spec.TargetBranch
	dst.Spec.MergeStrategy = v1beta3.MergeStrategy(src.Spec.MergeStrategy)

	return nil
}

func (dst *RemoteTarget) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.RemoteTarget)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.UpstreamRepository = src.Spec.UpstreamRepository
	dst.Spec.UpstreamBranch = src.Spec.UpstreamBranch
	dst.Spec.TargetRepository = src.Spec.TargetRepository
	dst.Spec.TargetBranch = src.Spec.TargetBranch
	dst.Spec.MergeStrategy = MergeStrategy(src.Spec.MergeStrategy)

	return nil
}
