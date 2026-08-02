package render

import (
	"context"

	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

// ExcludedFieldsFromConfigMap reads the excludedFields list of a ConfigMap.
func ExcludedFieldsFromConfigMap(
	ctx context.Context,
	configMapName string,
	configMapNamespace string,
) ([]string, error) {
	k8sClient := kube.ClientFromContext(ctx)
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

// Remove the specified path from the json object
// Path examples :
//
// INPUT
//
//	test1.test2
//
// OUTPUT
//
//	test1:
//	  test2: value
//
// INPUT
//
//	.test3
//	test3: value
//
// OUTPUT
//
//	test7
//	test7: value
//
// # INPUT
//
// .test4[this.string-is:the/same*key]test5[test6]
//
// OUTPUT
//
//	test4:
//	"this.string-is:the/same*key":
//	  test5:
//	    test6: value
func RemoveExcludedField(data map[string]interface{}, path string) {
	parts := make([]string, 0)
	var current string
	inBrackets := false

	for _, char := range path {
		switch char {
		case '.':
			if !inBrackets {
				if current != "" {
					parts = append(parts, current)
				}
				current = ""
			} else {
				current += string(char)
			}
		case '[':
			inBrackets = true
			if current != "" {
				parts = append(parts, current)
			}
			current = ""
		case ']':
			inBrackets = false
			if current != "" {
				parts = append(parts, current)
			}
			current = ""
		default:
			current += string(char)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	if len(parts) == 0 {
		return
	}

	last := len(parts) - 1
	currentMap := data

	for i, part := range parts {
		if i == last {
			// Delete the last part from the current map
			delete(currentMap, part)
			return
		}

		// Traverse deeper
		val, ok := currentMap[part]
		if !ok {
			// Path not found
			return
		}
		nextMap, ok := val.(map[string]interface{})
		if !ok {
			// Can't descend further, not a map
			return
		}
		currentMap = nextMap
	}
}
