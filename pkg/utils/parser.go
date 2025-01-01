package utils

import (
	"fmt"

	admissionv1 "k8s.io/api/admissionregistration/v1"
)

// Remove the specified path from the json object
// Path examples :

//  test1.test2
//  test1:
//    test2: value

//  .test3
//  test3: value

//  test7
//  test7: value

// .test4[this.string-is:the/same*key]test5[test6]
/*
    test4:
	  "this.string-is:the/same*key":
	    test5:
	      test6: value
*/
func ExcludedFieldsFromJson(data map[string]interface{}, path string) {
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
	last := len(parts) - 1

	// Traverse the map based on the path
	for i, part := range parts {
		if i == last {
			// Last part of the path, delete the field
			delete(data, part)
			return
		}
		// Move to the next level of the map
		val, ok := data[part]
		if !ok {
			// Path not found
			return
		}
		// Check if the value is a map
		next, ok := val.(map[string]interface{})
		if !ok {
			// Not a map, cannot traverse further
			return
		}
		// Update data for next iteration
		data = next
	}
}

func OperationToVerb(operation admissionv1.OperationType) ([]string, error) {
	switch operation {
	case admissionv1.Create:
		return []string{"create"}, nil
	case admissionv1.Delete:
		return []string{"delete"}, nil
	case admissionv1.Update:
		return []string{"update", "patch"}, nil
	case admissionv1.Connect:
		return []string{"connect"}, nil
	default:
		return nil, fmt.Errorf("unsupported operation: %v", operation)
	}
}
