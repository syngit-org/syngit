package mutator

import (
	"fmt"
	"strings"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/go-git/go-git/v5"
	fluxprovider "github.com/syngit-org/syngit-provider-flux/pkg"
	helmprovider "github.com/syngit-org/syngit-provider-helm/pkg"
	"github.com/syngit-org/syngit/pkg/interceptor"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
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

	ref, worktreePath, ok, err := discoverSourceRef(rc, secret)
	if err != nil {
		return err
	}
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

	// When the existing HelmRelease was found in the repo, overwrite it in place at
	// its own path: ResourceFinder cannot match it (it keys on the intercepted
	// secret name, not the HelmRelease's release name). When it came from the
	// cluster, fall back to the default structured placement.
	out.Add(Artifact{GVR: helmReleaseGVR, Content: []byte(result.RawYAML), TargetPath: worktreePath})

	return nil
}

// discoverSourceRef finds the chart sourceRef by locating the already-existing
// Flux HelmRelease of the same release (same name and namespace as the
// intercepted Helm release) and copying its spec.chart.spec.sourceRef. It looks
// in the git worktree first and, when nothing matches and a cluster reader is
// available, falls back to the live cluster. The returned path is the
// worktree-relative path of the matched HelmRelease when found in the repo (so
// the caller can overwrite it in place) and empty for the cluster fallback. The
// boolean is false when no matching HelmRelease (with a sourceRef) can be found.
func discoverSourceRef(rc RenderContext, secret *corev1.Secret) (helmv2.CrossNamespaceObjectReference, string, bool, error) {
	rel, err := helmprovider.ExtractRelease(secret)
	if err != nil || rel == nil || rel.Name == "" {
		return helmv2.CrossNamespaceObjectReference{}, "", false, nil
	}
	name, namespace := rel.Name, rel.Namespace

	// Repo first
	ref, path, found, err := sourceRefInWorktree(rc.Worktree, name, namespace)
	if err != nil {
		return helmv2.CrossNamespaceObjectReference{}, "", false, err
	}
	if found {
		return ref, path, true, nil
	}

	// Cluster fallback
	if rc.Cluster == nil {
		return helmv2.CrossNamespaceObjectReference{}, "", false, nil
	}

	ref, found, err = sourceRefInCluster(rc, name, namespace)
	return ref, "", found, err
}

// sourceRefInWorktree walks the worktree for a HelmRelease matching name (and
// namespace, when set on the manifest) and returns its sourceRef together with
// the worktree-relative path of the file that holds it.
func sourceRefInWorktree(worktree *git.Worktree, name, namespace string) (helmv2.CrossNamespaceObjectReference, string, bool, error) {
	if worktree == nil {
		return helmv2.CrossNamespaceObjectReference{}, "", false, nil
	}

	var (
		ref   helmv2.CrossNamespaceObjectReference
		path  string
		found bool
	)

	root := worktree.Filesystem.Root()
	err := WalkWorktreeYAML(worktree, root, func(docPath string, content []byte) (bool, bool, error) {
		doc := map[string]interface{}{}
		if uerr := yaml.Unmarshal(content, &doc); uerr != nil {
			return false, true, uerr
		}
		if !fluxprovider.IsCorrectHelmRelease(doc, name, namespace) {
			return false, true, nil
		}
		if r, ok := fluxprovider.ExtractSourceRefFromHelmRelease(doc); ok {
			ref, found = r, true
			path = relativeWorktreePath(root, docPath)
			return true, false, nil
		}
		return false, false, nil
	})
	if err != nil {
		return helmv2.CrossNamespaceObjectReference{}, "", false, err
	}

	return ref, path, found, nil
}

// relativeWorktreePath turns a path produced by the worktree walk into one
// relative to the worktree root, mirroring how the placement phase records
// claimed paths (no leading slash).
func relativeWorktreePath(root, path string) string {
	rel := strings.TrimPrefix(path, root)
	return strings.TrimPrefix(rel, "/")
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
