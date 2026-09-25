package mutator

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/go-git/go-billy/v5/helper/iofs"
	kustomizeprovider "github.com/syngit-org/syngit-provider-kustomize/pkg"
	"github.com/syngit-org/syngit/internal/walker"
	features "github.com/syngit-org/syngit/pkg/feature"
	"github.com/syngit-org/syngit/pkg/interceptor"
)

type KustomizeProvider struct{}

// Handles matches every object of a kustomize-enabled syncer that also runs the
// ResourceFinder, which is how the files of a bundle are located. The Kustomize
// feature gate is not checked here: providerGate is keyed by it, so this runs
// only when it is on. Whether an object is really kustomize-managed is decided
// in Render, which reads its annotations.
func (KustomizeProvider) Handles(params interceptor.GitPipelineParams) bool {
	return params.Syncer.Spec.Kustomize.Enabled &&
		features.LoadedFeatureGates.Enabled(features.ResourceFinder) &&
		params.Syncer.Spec.ResourceFinder
}

// Render turns the intercepted resource into its base resource or its overlay
// patch and emits it at the path the bundle dictates. It produces nothing for an
// object carrying no bundle-path: that object is not kustomize-managed and flows
// through the normal placement.
func (p KustomizeProvider) Render(rc RenderContext, out *ArtifactSet) error {
	params := rc.Params
	annotations := params.InterceptedAnnotations

	bundlePath := annotations[kustomizeprovider.BundlePathAnnotation]
	if bundlePath == "" {
		return nil
	}

	config, err := kustomizeConfig(params)
	if err != nil {
		return err
	}
	overlayOnly, err := overlayOnly(annotations)
	if err != nil {
		return err
	}

	bundle, err := rc.Worktree.Filesystem.Chroot(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to open the kustomize bundle %s: %w", bundlePath, err)
	}
	bundleFS := iofs.NewReadDirFS(bundle)

	overlay, err := kustomizeprovider.DetectOverlay(bundleFS, config)
	if err != nil {
		return fmt.Errorf("failed to detect the overlay of the kustomize bundle %s: %w", bundlePath, err)
	}

	if config.BaseName == "" {
		config.BaseName = strings.TrimSuffix(
			strings.TrimPrefix(params.InterceptedName, overlay.Kustomization.NamePrefix),
			overlay.Kustomization.NameSuffix,
		)
	}

	baseDir := ""
	if !overlayOnly {
		detected, err := kustomizeprovider.DetectBase(bundleFS, overlay)
		if err != nil {
			return fmt.Errorf("failed to detect the base of the kustomize bundle %s: %w", bundlePath, err)
		}
		baseDir = path.Join(bundlePath, detected)
	}

	targetDir := path.Join(bundlePath, overlay.Path)
	if overlayOnly {
		// An overlay-only resource has no base to differ from: it is written into
		// the overlay whole, stripped of what the overlay injected.
		config.Override = kustomizeprovider.Base
	} else if config.Override == kustomizeprovider.Base {
		targetDir = baseDir
	}

	// The file already holding the resource wins; a resource the bundle does not
	// hold yet gets a file of its own.
	existingPath, existingDoc, found := findInBundle(rc, targetDir, config.BaseName)
	targetPath := path.Join(targetDir, config.BaseName+".yaml")
	if found {
		targetPath = existingPath
	}

	if params.InterceptedYAML == "" {
		out.Add(Artifact{TargetPath: targetPath})
		return nil
	}

	var baseResource, existingPatch []byte
	if config.Override == kustomizeprovider.Overlay {
		baseResource, err = readBase(rc, baseDir, config.BaseName)
		if err != nil {
			return err
		}
		existingPatch = existingDoc
	}

	content, err := kustomizeprovider.Convert(
		config, []byte(params.InterceptedYAML), baseResource, overlay.Raw, existingPatch,
	)
	if err != nil {
		return fmt.Errorf("failed to convert %s back into its kustomize bundle: %w", params.InterceptedName, err)
	}

	out.Add(Artifact{TargetPath: targetPath, Content: content})
	return nil
}

// kustomizeConfig assembles the provider configuration from the syncer spec and
// the annotations of the intercepted object.
//
// Bundle is left empty: the overlay annotation selects the overlay, and the
// labels the overlay's own kustomization declares are stripped without it. A
// label carrying the bundle value but declared further up is left in place.
func kustomizeConfig(params interceptor.GitPipelineParams) (kustomizeprovider.KustomizeProviderConfig, error) {
	config := kustomizeprovider.KustomizeProviderConfig{
		Override:      params.Syncer.Spec.Kustomize.Override,
		PatchStrategy: kustomizeprovider.StrategicMerge,
		BaseName:      params.InterceptedAnnotations[kustomizeprovider.BaseNameAnnotation],
		Overlay:       params.InterceptedAnnotations[kustomizeprovider.OverlayAnnotation],
	}

	switch strategy := params.InterceptedAnnotations[kustomizeprovider.PatchStrategyAnnotation]; strategy {
	case "":
	case string(kustomizeprovider.StrategicMerge), string(kustomizeprovider.JSON6902):
		config.PatchStrategy = kustomizeprovider.PatchStrategy(strategy)
	default:
		return config, fmt.Errorf("%s must be %s or %s, got %q",
			kustomizeprovider.PatchStrategyAnnotation,
			kustomizeprovider.StrategicMerge, kustomizeprovider.JSON6902, strategy)
	}

	return config, nil
}

func overlayOnly(annotations map[string]string) (bool, error) {
	value := annotations[kustomizeprovider.OverlayOnlyAnnotation]
	if value == "" {
		return false, nil
	}
	only, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean, got %q", kustomizeprovider.OverlayOnlyAnnotation, value)
	}
	return only, nil
}

// readBase returns the base resource Convert differs the intercepted one
// against. A bundle without it cannot produce a patch.
func readBase(rc RenderContext, dir, name string) ([]byte, error) {
	_, doc, found := findInBundle(rc, dir, name)
	if !found {
		return nil, fmt.Errorf("failed to find the base %s of %s in %s", name, rc.Params.InterceptedName, dir)
	}
	return doc, nil
}

// findInBundle walks dir for the document carrying the resource under name and
// returns it with the worktree path of the file holding it. The namespace is
// ignored: a base rarely carries the one its overlay injects.
func findInBundle(rc RenderContext, dir, name string) (string, []byte, bool) {
	gvr := rc.Params.InterceptedGVR
	root := rc.Worktree.Filesystem.Root()
	foundPath, foundDoc := "", []byte(nil)

	err := walker.WalkWorktreeYAML(rc.Worktree, path.Join(root, dir), func(p string, doc []byte) (bool, bool, error) {
		sel := walker.SelectorFromDoc(doc)
		if sel.Name != name || sel.GVR.Group != gvr.Group || sel.GVR.Resource != gvr.Resource {
			return false, true, nil
		}
		foundPath = strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
		foundDoc = append([]byte(nil), doc...)
		return true, false, nil
	})

	return foundPath, foundDoc, err == nil && foundPath != ""
}
