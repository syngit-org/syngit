package utils

import (
	"context"
	"encoding/json"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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
func ConvertObjectJSONToYAMLString(
	ctx context.Context,
	rawObject []byte,
	syngitNamespace string,
	spec syngit.RemoteSyncerSpec,
	refOwnerNamespace string,
) (string, error) {
	data, err := ConvertObjectJSONToYAMLMap(rawObject)
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

	k8sClient := K8sClientFromContext(ctx)

	err = k8sClient.List(ctx, &defaultExcludedFieldsCms, listOps)
	if err != nil {
		return "", err
	}
	for _, defaultExcludedFieldsCm := range defaultExcludedFieldsCms.Items {
		excludedFieldsFromCm, err := GetExcludedFieldsFromConfigMap(
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
		cmNamespace, err := ResolveNamespace(
			ref.Namespace,
			refOwnerNamespace,
			field.NewPath("spec", "excludedFieldsConfigMapsRef").Index(i),
		)
		if err != nil {
			return "", err
		}
		excludedFieldsFromCm, err := GetExcludedFieldsFromConfigMap(
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
		ExcludedFieldsFromJson(data, path)
	}

	// Marshal back to YAML
	updatedYAML, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}

	return string(updatedYAML), nil
}

func GetExcludedFieldsFromConfigMap(
	ctx context.Context,
	configMapName string,
	configMapNamespace string,
) ([]string, error) {
	k8sClient := K8sClientFromContext(ctx)
	namespacedName := types.NamespacedName{Namespace: configMapNamespace, Name: configMapName}

	excludedFieldsConfig := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, namespacedName, excludedFieldsConfig)
	if err != nil {
		return nil, err
	}
	yamlString := excludedFieldsConfig.Data["excludedFields"]
	var excludedFields []string

	// Unmarshal the YAML string into the Go array
	err = yaml.Unmarshal([]byte(yamlString), &excludedFields)
	if err != nil {
		return nil, syngiterrors.NewWrongYAMLFormat("failed to convert the excludedFields from the ConfigMap")
	}

	return excludedFields, nil
}

func ConvertObjectJSONToYAMLMap(rawObject []byte) (map[string]interface{}, error) {
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

type ObjectMetadata struct {
	GVR schema.GroupVersionResource
	// Name of the intercepted object.
	Name string
	// Namespace of the intercepted object. Empty when it is cluster-scoped.
	Namespace string
}

func ExtractObjectMetadataFromAdmissionRequest(admissionRequest *admissionv1.AdmissionRequest) ObjectMetadata {
	interceptedGVR := (*schema.GroupVersionResource)(admissionRequest.RequestResource.DeepCopy())

	return ObjectMetadata{
		Name:      admissionRequest.Name,
		Namespace: admissionRequest.Namespace,
		GVR:       *interceptedGVR,
	}
}
