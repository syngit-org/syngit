package mutator

import (
	provider "github.com/syngit-org/syngit-provider-helm/pkg"
	"github.com/syngit-org/syngit/pkg/interceptor"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	yaml "k8s.io/apimachinery/pkg/util/yaml"
)

type HelmValuesMutation struct{}

func (mutate HelmValuesMutation) Mutate(params interceptor.GitPipelineParams, mutations *Mutations) error {
	if params.InterceptedGVR.Group != "" ||
		params.InterceptedGVR.Version != "v1" ||
		params.InterceptedGVR.Resource != "secrets" {
		return nil
	}

	content := []byte(params.InterceptedYAML)

	secret := &corev1.Secret{}
	if err := yaml.Unmarshal(content, secret); err != nil {
		return err
	}

	if !provider.IsHelmSecret(secret) {
		return nil
	}

	values, err := provider.ExtractValues(secret)
	if err != nil {
		return err
	}
	_ = values

	mutations.AddMutation(schema.GroupVersionResource{
		Group:    "",
		Version:  "",
		Resource: "chart-values",
	}, content)

	return nil
}
