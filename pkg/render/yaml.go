package render

import (
	"context"
	"encoding/json"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"github.com/syngit-org/syngit/pkg/kube"
	"github.com/syngit-org/syngit/pkg/refs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// Convert the json string object to a yaml string.
// We have no other choice than extracting the json into a map
// and then convert the map into a yaml string.
// Because the 'map' object is, by definition, not ordered
// we cannot reorder fields.
// refOwnerNamespace is the syncer's namespace (empty for CWRSY)
func ObjectToYAML(
	ctx context.Context,
	rawObject []byte,
	syngitNamespace string,
	spec syngit.RemoteSyncerSpec,
	refOwnerNamespace string,
) (string, error) {
	data, err := JSONToMap(rawObject)
	if err != nil {
		return "", err
	}

	// Excluded fields paths to remove
	paths := []string{}

	// Search for cluster default excluded fields
	defaultExcludedFieldsCms := corev1.ConfigMapList{}
	listOps := &client.ListOptions{
		Namespace: syngitNamespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"syngit.io/cluster-default-excluded-fields": "true",
		}),
	}

	k8sClient := kube.ClientFromContext(ctx)

	err = k8sClient.List(ctx, &defaultExcludedFieldsCms, listOps)
	if err != nil {
		return "", err
	}
	for _, defaultExcludedFieldsCm := range defaultExcludedFieldsCms.Items {
		excludedFieldsFromCm, err := ExcludedFieldsFromConfigMap(
			ctx,
			defaultExcludedFieldsCm.Name,
			defaultExcludedFieldsCm.Namespace,
		)
		if err != nil {
			return "", err
		}
		paths = append(paths, excludedFieldsFromCm...)
	}

	// excludedFields hardcoded in the syncer
	excludedFieldsFromRsy := spec.ExcludedFields
	paths = append(paths, excludedFieldsFromRsy...)

	// Loop over the excluded fields ConfigMaps
	for i, ref := range spec.ExcludedFieldsConfigMapsRef {
		if ref == nil {
			continue
		}
		cmNamespace, err := refs.ResolveNamespace(
			ref.Namespace,
			refOwnerNamespace,
			field.NewPath("spec", "excludedFieldsConfigMapsRef").Index(i),
		)
		if err != nil {
			return "", err
		}
		excludedFieldsFromCm, err := ExcludedFieldsFromConfigMap(
			ctx,
			ref.Name,
			cmNamespace,
		)
		if err != nil {
			return "", err
		}
		paths = append(paths, excludedFieldsFromCm...)
	}

	// Remove unwanted fields
	for _, path := range paths {
		RemoveExcludedField(data, path)
	}

	// Marshal back to YAML
	updatedYAML, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}

	return string(updatedYAML), nil
}

func JSONToMap(rawObject []byte) (map[string]interface{}, error) {
	var data map[string]interface{}
	err := json.Unmarshal(rawObject, &data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func ContainsDeletionTimestamp(data map[string]interface{}) bool {
	metadata, _ := data["metadata"].(map[string]interface{})
	_, ok := metadata["deletionTimestamp"]
	return ok
}
