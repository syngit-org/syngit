package mutator

import (
	"fmt"
	"strings"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	fluxprovider "github.com/syngit-org/syngit-provider-flux/pkg"
	helmprovider "github.com/syngit-org/syngit-provider-helm/pkg"
	"github.com/syngit-org/syngit/pkg/interceptor"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
// Flux v2 HelmRelease. The chart sourceRef is not stored in the secret, so
// the provider copies it from the already-existing HelmRelease of the same
// release: it looks in the git repo first and falls back to the live cluster.
type FluxHelmReleaseProvider struct{}

// Handles matches Helm release Secrets, mirroring HelmValuesMutation.Handles.
func (FluxHelmReleaseProvider) Handles(params interceptor.GitPipelineParams) bool {
	if params.InterceptedGVR.Group != "" ||
		params.InterceptedGVR.Version != "v1" ||
		params.InterceptedGVR.Resource != "secrets" {
		return false
	}
	return helmprovider.IsHelmSecretByName(params.InterceptedName)
}

// Render synthesizes a Flux HelmRelease from the intercepted Helm release Secret,
// copying the chart sourceRef from the already-existing HelmRelease of the same
// release (discovered in the git repo, then the cluster).
func (p FluxHelmReleaseProvider) Render(rc RenderContext, out *ArtifactSet) error {
	params := rc.Params

	// Deletion: emit an empty (deletion) HelmRelease artifact, keyed by the
	// secret name through the placement phase. Mirrors HelmValuesMutation so the
	// create/delete paths stay symmetric.
	if params.InterceptedYAML == "" {
		out.Add(Artifact{GVR: helmReleaseGVR, Content: []byte("")})
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
		return fmt.Errorf("the cluster client is not set")
	}

	ref, ok, err := sourceRefInCluster(rc, name, namespace)
	if !ok {
		// No HelmRepository to source from: a sourceRef-less HelmRelease would be
		// invalid for Flux, so leave the secret to other providers / the default
		// seed rather than emitting a broken resource.
		return nil
	}

	result, err := fluxprovider.ConvertToHelmRelease(secret, ref)
	if err != nil {
		return fmt.Errorf("failed to convert the Helm release secret to a HelmRelease: %w", err)
	}

	out.Add(Artifact{GVR: helmReleaseGVR, Content: []byte(result.RawYAML)})

	return nil
}

// sourceRefInCluster lists HelmReleases from the cluster (probing the served
// apiVersion), finds the one matching name/namespace, and returns its sourceRef.
func sourceRefInCluster(rc RenderContext, name, namespace string) (helmv2.CrossNamespaceObjectReference, bool, error) {
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
			return helmv2.CrossNamespaceObjectReference{}, false,
				fmt.Errorf("failed to list HelmReleases: %w", err)
		}

		for i := range list.Items {
			item := &list.Items[i]
			if item.GetName() != name || item.GetNamespace() != namespace {
				continue
			}
			if ref, ok := fluxprovider.ExtractSourceRefFromHelmRelease(item.Object); ok {
				return ref, true, nil
			}
		}
	}

	return helmv2.CrossNamespaceObjectReference{}, false, nil
}
