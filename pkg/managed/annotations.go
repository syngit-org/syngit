package managed

import (
	"strings"
)

// BranchesFromAnnotation parses the comma-separated branch list carried by a
// policy annotation. A blank annotation yields no branch at all.
func BranchesFromAnnotation(in string) []string {
	out := strings.Split(strings.ReplaceAll(in, " ", ""), ",")
	if len(out) == 1 && out[0] == "" {
		return []string{}
	}
	return out
}
