/*
Copyright 2026.

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
	"github.com/syngit-org/syngit/pkg/api/v1beta5"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *RemoteTarget) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta5.RemoteTarget)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.UpstreamRepository = src.Spec.UpstreamRepository
	dst.Spec.UpstreamBranch = src.Spec.UpstreamBranch
	dst.Spec.TargetRepository = src.Spec.TargetRepository
	dst.Spec.TargetBranch = src.Spec.TargetBranch
	dst.Spec.MergeStrategy = v1beta5.MergeStrategy(src.Spec.MergeStrategy)

	// Status conversion - preserve existing status from hub
	// Only convert if src has status set (for backward compatibility)
	if len(src.Status.Conditions) > 0 {
		dst.Status.Conditions = src.Status.Conditions
	}

	return nil
}

func (dst *RemoteTarget) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta5.RemoteTarget)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.UpstreamRepository = src.Spec.UpstreamRepository
	dst.Spec.UpstreamBranch = src.Spec.UpstreamBranch
	dst.Spec.TargetRepository = src.Spec.TargetRepository
	dst.Spec.TargetBranch = src.Spec.TargetBranch
	dst.Spec.MergeStrategy = MergeStrategy(src.Spec.MergeStrategy)

	// Status conversion - preserve existing status from hub
	// Only convert if src has status set (for backward compatibility)
	if len(src.Status.Conditions) > 0 {
		dst.Status.Conditions = src.Status.Conditions
	}

	return nil
}
