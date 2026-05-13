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

func (mutate HelmValuesMutation) Mutate(params interceptor.GitPipelineParams, mutations *Mutations) error {
	if params.InterceptedGVR.Group != "" ||
		params.InterceptedGVR.Version != "v1" ||
		params.InterceptedGVR.Resource != "secrets" {
		return nil
	}

	if !provider.IsHelmSecretByName(params.InterceptedName) {
		return nil
	}

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

	mutations.AddMutation(schema.GroupVersionResource{
		Group:    "",
		Version:  "",
		Resource: DefaultChartValuesSubPath,
	}, []byte(rawValues))

	return nil
}
