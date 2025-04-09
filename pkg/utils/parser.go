package utils

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
