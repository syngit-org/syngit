package mutator

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	fluxprovider "github.com/syngit-org/syngit-provider-flux/pkg"
	helmprovider "github.com/syngit-org/syngit-provider-helm/pkg"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"github.com/syngit-org/syngit/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

const (
	fluxHelmGroup   = "helm.toolkit.fluxcd.io"
	helmReleaseKind = "HelmRelease"
)

// helmReleaseGVR is the identity of the synthesized Flux HelmRelease artifact.
var helmReleaseGVR = schema.GroupVersionResource{
	Group:    fluxHelmGroup,
	Version:  "v2",
	Resource: "helmreleases",
}

// helmReleaseVersions are the apiVersions tried, newest first, when listing
// HelmReleases in the cluster (their served version is not known upfront).
var helmReleaseVersions = []string{"v2", "v2beta2", "v2beta1"}

// FluxHelmReleaseProvider turns an intercepted Helm release Secret into a
// Flux v2 HelmRelease. The Helm release secret only carries the chart and the
// user-supplied values, so the provider copies the already-existing HelmRelease
// of the same release from the live cluster and overrides only its spec.values
// with the values from the secret. Every other field (chart, sourceRef,
// interval, install/upgrade options, ...) is preserved from the live resource.
type FluxHelmReleaseProvider struct{}

// Handles matches Helm release Secrets, mirroring HelmValuesMutation.Handles.
func (FluxHelmReleaseProvider) Handles(params interceptor.GitPipelineParams) bool {
	if params.RemoteSyncer.Annotations[fluxprovider.HelmReleaseAnnotation] != "enabled" ||
		params.InterceptedGVR.Group != "" ||
		params.InterceptedGVR.Version != "v1" ||
		params.InterceptedGVR.Resource != "secrets" {
		return false
	}
	return helmprovider.IsHelmSecretByName(params.InterceptedName)
}

// Render synthesizes a Flux HelmRelease from the intercepted Helm release Secret.
// It copies the already-existing HelmRelease of the same release from the live
// cluster, overrides its spec.values with the secret's user-supplied values, and
// strips the RemoteSyncer's excluded fields before emitting the artifact.
func (p FluxHelmReleaseProvider) Render(rc RenderContext, out *ArtifactSet) error {
	params := rc.Params

	// Deletion: emit an empty (deletion) HelmRelease artifact. The secret body is
	// gone on a delete, but its name embeds the release name, so the placement
	// phase can still locate the HelmRelease by its real identity (the release
	// name, not the secret name). The release lives in the same namespace as the
	// intercepted secret.
	if params.InterceptedYAML == "" {
		out.Add(Artifact{
			GVR:       helmReleaseGVR,
			Name:      helmprovider.GetReleaseNameFromSecretName(params.InterceptedName),
			Namespace: params.RemoteSyncer.Namespace,
			Content:   []byte(""),
		})
		return nil
	}

	secret := &corev1.Secret{}
	if err := utilyaml.Unmarshal([]byte(params.InterceptedYAML), secret); err != nil {
		return fmt.Errorf("failed to parse the Helm release secret: %w", err)
	}
	if !helmprovider.IsHelmSecret(secret) {
		return nil
	}

	rel, err := helmprovider.ExtractRelease(secret)
	if err != nil || rel == nil || rel.Name == "" {
		return err
	}
	name, namespace := rel.Name, rel.Namespace
	if rc.Cluster == nil {
		// Without a cluster reader the existing HelmRelease can't be looked up, so
		// decline gracefully and let other providers / the default seed handle the
		// secret rather than failing the whole pipeline.
		return nil
	}

	existing, ok, err := existingHelmReleaseInCluster(rc, name, namespace)
	if err != nil {
		return err
	}
	if !ok {
		// No live HelmRelease to copy from: without an existing resource (and its
		// sourceRef) a synthesized HelmRelease would be invalid for Flux, so leave
		// the secret to other providers / the default seed rather than emitting a
		// broken resource.
		return nil
	}

	result, err := fluxprovider.ConvertToHelmReleaseWithExisting(secret, existing)
	if err != nil {
		return fmt.Errorf("failed to convert the Helm release secret to a HelmRelease: %w", err)
	}

	// ConvertToHelmReleaseWithExisting copies the live resource verbatim, including
	// server-managed metadata/status. Strip the RemoteSyncer's excluded fields, the
	// same way the intercepted object is cleaned, before writing to git.
	raw, err := json.Marshal(result.HelmRelease)
	if err != nil {
		return fmt.Errorf("failed to marshal the generated HelmRelease: %w", err)
	}
	cleaned, err := utils.ConvertObjectJSONToYAMLString(rc.Ctx, raw, os.Getenv("MANAGER_NAMESPACE"), rc.Params.RemoteSyncer)
	if err != nil {
		return fmt.Errorf("failed to apply the excluded fields to the HelmRelease: %w", err)
	}

	out.Add(Artifact{
		GVR:       helmReleaseGVR,
		Name:      name,
		Namespace: namespace,
		Content:   []byte(cleaned),
	})

	return nil
}

// existingHelmReleaseInCluster lists HelmReleases from the cluster (probing the
// served apiVersion), finds the one matching name/namespace, decodes it into a
// typed HelmRelease, and returns it. The boolean is false when no matching
// HelmRelease with a chart sourceRef is found: a sourceRef-less HelmRelease would
// be invalid for Flux, so callers should not emit one.
func existingHelmReleaseInCluster(rc RenderContext, name, namespace string) (*helmv2.HelmRelease, bool, error) {
	for _, version := range helmReleaseVersions {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   fluxHelmGroup,
			Version: version,
			Kind:    helmReleaseKind + "List",
		})

		if err := rc.Cluster.List(rc.Ctx, list); err != nil {
			// The version may not be served by the cluster; try the next one.
			if msg := err.Error(); strings.Contains(msg, "no matches for kind") ||
				strings.Contains(msg, "could not find the requested resource") {
				continue
			}
			return nil, false, fmt.Errorf("failed to list HelmReleases: %w", err)
		}

		for i := range list.Items {
			item := &list.Items[i]
			if item.GetName() != name || item.GetNamespace() != namespace {
				continue
			}
			hr := &helmv2.HelmRelease{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, hr); err != nil {
				return nil, false, fmt.Errorf("failed to decode the HelmRelease %s/%s: %w", namespace, name, err)
			}
			// A sourceRef-less HelmRelease would be invalid for Flux; keep scanning.
			if hr.Spec.Chart == nil || hr.Spec.Chart.Spec.SourceRef.Name == "" {
				continue
			}
			return hr, true, nil
		}
	}

	return nil, false, nil
}
