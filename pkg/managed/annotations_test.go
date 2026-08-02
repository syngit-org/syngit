package managed

import (
	"testing"
)

func TestBranchesFromAnnotation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string returns empty slice", "", []string{}},
		{"single branch", "main", []string{"main"}},
		{"comma-separated branches", "main,dev", []string{"main", "dev"}},
		{"whitespace is stripped", "main, dev , feature", []string{"main", "dev", "feature"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BranchesFromAnnotation(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
