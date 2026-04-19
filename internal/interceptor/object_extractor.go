package interceptor

import (
	"context"
	"encoding/json"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/pkg/utils"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// Convert the json string object to a yaml string.
// We have no other choice than extracting the json into a map
// and then convert the map into a yaml string.
// Because the 'map' object is, by definition, not ordered
// we cannot reorder fields.
func ConvertObjectJSONToYAMLString(
	ctx context.Context,
	rawObject []byte,
	syngitNamespace string,
	remoteSyncer syngit.RemoteSyncer,
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

	// excludedFields hardcoded in RemoteSyncer
	excludedFieldsFromRsy := remoteSyncer.Spec.ExcludedFields
	paths = append(paths, excludedFieldsFromRsy...)

	// Check if the excludedFields ConfigMap exists
	if remoteSyncer.Spec.ExcludedFieldsConfigMapRef != nil && remoteSyncer.Spec.ExcludedFieldsConfigMapRef.Name != "" {
		excludedFieldsFromCm, err := GetExcludedFieldsFromConfigMap(
			ctx,
			remoteSyncer.Spec.ExcludedFieldsConfigMapRef.Name,
			remoteSyncer.Namespace,
		)
		if err != nil {
			return "", err
		}
		paths = append(paths, excludedFieldsFromCm...)
	}

	// Remove unwanted fields
	for _, path := range paths {
		utils.ExcludedFieldsFromJson(data, path)
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
	GVR  schema.GroupVersionResource
	Name string
}

func ExtractObjectMetadataFromAdmissionRequest(admissionRequest *admissionv1.AdmissionRequest) ObjectMetadata {
	interceptedGVR := (*schema.GroupVersionResource)(admissionRequest.RequestResource.DeepCopy())

	return ObjectMetadata{
		Name: admissionRequest.Name,
		GVR:  *interceptedGVR,
	}
}
