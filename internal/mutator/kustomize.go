package mutator

import (
	"path"
	"reflect"
	"slices"

	"github.com/go-git/go-billy/v5/helper/iofs"
	"github.com/go-git/go-git/v5"
	kustomizeprovider "github.com/syngit-org/syngit-provider-kustomize/pkg"
	"github.com/syngit-org/syngit/internal/walker"
	features "github.com/syngit-org/syngit/pkg/feature"
	"github.com/syngit-org/syngit/pkg/interceptor"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
	"sigs.k8s.io/yaml"
)

type KustomizeProvider struct{}

// Handles only checks the syncer: whether the object belongs to a kustomize
// bundle is decided in Render, from its labels.
func (KustomizeProvider) Handles(params interceptor.GitPipelineParams) bool {
	return params.Syncer.Annotations[kustomizeprovider.ProviderAnnotation] == providerEnabled
}

func (KustomizeProvider) Render(rc RenderContext, out *ArtifactSet) error {
	obj, err := interceptedObject(rc.Params)
	if err != nil {
		return err
	}

	resourceFinder := features.LoadedFeatureGates.Enabled(features.ResourceFinder) && rc.Params.Syncer.Spec.ResourceFinder
	result, err := kustomizeprovider.Process(obj, rc.Params.Operation, iofs.New(rc.Worktree.Filesystem), resourceFinder)
	if err != nil {
		return err
	}
	if !result.Handled {
		return nil
	}

	out.Claimed = true
	if result.Decision.Action == kustomizeprovider.ActionRefuse {
		out.Denial = result.Decision.RefuseMessage
		return nil
	}

	for file, content := range result.Files {
		// SOPS keeps its metadata in a top-level mapping, which a JSON6902 patch
		// (a list) cannot hold.
		if rc.Transform != nil && result.Decision.PatchStrategy != kustomizeprovider.PatchStrategyJSON6902 {
			existing, _ := walker.ReadWorktreeFile(rc.Worktree, file)
			content, err = walker.TransformChangedDocs(file, existing, content, rc.Transform)
			if err != nil {
				return err
			}
		}
		out.Add(Artifact{TargetPath: file, Content: content, WholeFile: true})
	}
	for _, file := range result.Deletes {
		out.Add(Artifact{TargetPath: file, WholeFile: true})
	}

	kustomizations, err := applyKustomizationEdits(rc.Worktree, result.Edits)
	if err != nil {
		return err
	}
	for file, content := range kustomizations {
		out.Add(Artifact{TargetPath: file, Content: content, WholeFile: true})
	}
	return nil
}

// The provider reads labels and annotations off the object, and on DELETE only
// the removed object carries them.
func interceptedObject(params interceptor.GitPipelineParams) (*unstructured.Unstructured, error) {
	manifest := params.InterceptedManifest
	if params.Operation == admissionv1.Delete {
		manifest = params.DeletedManifest
	}
	// Decoding from JSON gives the int64 numbers the provider compares against.
	content, err := yaml.YAMLToJSON([]byte(manifest))
	if err != nil {
		return nil, err
	}
	obj := &unstructured.Unstructured{}
	return obj, obj.UnmarshalJSON(content)
}

// kyaml keeps the comments, the key order and the sequence indentation of the
// kustomization files.
func applyKustomizationEdits(worktree *git.Worktree, edits []kustomizeprovider.Edit) (map[string][]byte, error) {
	contents := map[string][]byte{}
	for _, edit := range edits {
		content, ok := contents[edit.Kustomization]
		if !ok {
			var err error
			content, err = walker.ReadWorktreeFile(worktree, edit.Kustomization)
			if err != nil {
				return nil, err
			}
		}
		kustomization, err := kyaml.Parse(string(content))
		if err != nil {
			return nil, err
		}
		if err := applyKustomizationEdit(kustomization, edit); err != nil {
			return nil, err
		}
		seqIndent := kyaml.SequenceIndentStyle(kyaml.DeriveSeqIndentStyle(string(content)))
		contents[edit.Kustomization], err = kyaml.MarshalWithOptions(kustomization.Document(), &kyaml.EncoderOptions{SeqIndent: seqIndent})
		if err != nil {
			return nil, err
		}
	}
	return contents, nil
}

func applyKustomizationEdit(kustomization *kyaml.RNode, edit kustomizeprovider.Edit) error {
	entries, err := kustomization.Pipe(kyaml.LookupCreate(kyaml.SequenceNode, string(edit.Field)))
	if err != nil {
		return err
	}

	if edit.Op == kustomizeprovider.EditOpAdd {
		content, err := yaml.Marshal(edit.Entry)
		if err != nil {
			return err
		}
		entry, err := kyaml.Parse(string(content))
		if err != nil {
			return err
		}
		return entries.PipeE(kyaml.Append(entry.YNode()))
	}

	elements := entries.YNode().Content
	for i, element := range elements {
		content, err := kyaml.NewRNode(element).String()
		if err != nil {
			return err
		}
		var value any
		if err := yaml.Unmarshal([]byte(content), &value); err != nil {
			return err
		}
		// Like the provider, a resources: path is compared cleaned.
		if file, ok := value.(string); ok {
			value = path.Clean(file)
		}
		if reflect.DeepEqual(value, edit.Entry) {
			entries.YNode().Content = slices.Delete(elements, i, i+1)
			return nil
		}
	}
	return nil
}
