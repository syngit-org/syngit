package mutator

import (
	provider "github.com/syngit-org/syngit-provider-helm/pkg"
	"github.com/syngit-org/syngit/pkg/interceptor"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	yaml "k8s.io/apimachinery/pkg/util/yaml"
)

type HelmValuesMutation struct{}

const DefaultChartValuesSubPath = "chart-values"

// Handles matches Helm release Secrets by GVR and name.
func (mutate HelmValuesMutation) Handles(params interceptor.GitPipelineParams) bool {
	if params.InterceptedGVR.Group != "" ||
		params.InterceptedGVR.Version != "v1" ||
		params.InterceptedGVR.Resource != "secrets" {
		return false
	}

	return provider.IsHelmSecretByName(params.InterceptedName)
}

// Render extracts the chart values from the Helm release Secret and emits them
// as a single path-less artifact under the chart-values sub-path.
func (mutate HelmValuesMutation) Render(rc RenderContext, out *ArtifactSet) error {
	params := rc.Params
	rawValues := ""

	if params.InterceptedYAML != "" {
		secret := &corev1.Secret{}
		if err := yaml.Unmarshal([]byte(params.InterceptedYAML), secret); err != nil {
			return err
		}

		if !provider.IsHelmSecret(secret) {
			return nil
		}

		values, err := provider.ExtractValues(secret)
		if err != nil {
			return err
		}
		rawValues = values.RawValues
	}

	out.Add(Artifact{
		GVR: schema.GroupVersionResource{
			Group:    "",
			Version:  "",
			Resource: DefaultChartValuesSubPath,
		},
		Content: []byte(rawValues),
	})

	return nil
}
