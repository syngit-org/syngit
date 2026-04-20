package transformer

import (
	"testing"
)

func TestDefaultTransformer_validatePath(t *testing.T) {
	dt := DefaultTransformer{}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"simple path is cleaned and returned", "a/b/c", "a/b/c", false},
		{"trailing slash removed by Clean", "a/b/c/", "a/b/c", false},
		{"double-dot segments resolved by Clean", "a/b/../c", "a/c", false},
		{"colon is invalid", "a:b", "", true},
		{"asterisk is invalid", "a*b", "", true},
		{"question mark is invalid", "a?b", "", true},
		{"double-quote is invalid", "a\"b", "", true},
		{"less-than is invalid", "a<b", "", true},
		{"greater-than is invalid", "a>b", "", true},
		{"pipe is invalid", "a|b", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dt.validatePath(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("validatePath(%q)=%q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDefaultTransformer_containsInvalidCharacters(t *testing.T) {
	dt := DefaultTransformer{}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty string", "", false},
		{"valid alphanumeric path", "a/b/c-1.yaml", false},
		{"contains colon", "a:b", true},
		{"contains asterisk", "a*b", true},
		{"contains question mark", "a?b", true},
		{"contains double-quote", "a\"b", true},
		{"contains less-than", "a<b", true},
		{"contains greater-than", "a>b", true},
		{"contains pipe", "a|b", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := dt.containsInvalidCharacters(tc.input); got != tc.want {
				t.Errorf("containsInvalidCharacters(%q)=%v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestDefaultTransformer_getFileDirName(t *testing.T) {
	dt := DefaultTransformer{}

	tests := []struct {
		name         string
		resourceName string
		path         string
		filename     string
		wantDir      string
		wantFile     string
	}{
		{
			name:         "empty filename returns path-with-slash and <resource>.yaml",
			resourceName: "demo",
			path:         "foo/bar",
			filename:     "",
			wantDir:      "foo/bar/",
			wantFile:     "demo.yaml",
		},
		{
			name:         "non-empty filename with path ending in .yaml uses the path's last segment",
			resourceName: "demo",
			path:         "foo/bar.yaml",
			filename:     "some-other-name",
			wantDir:      "foo",
			wantFile:     "bar.yaml",
		},
		{
			name:         "non-empty filename with path ending in .yml uses the path's last segment",
			resourceName: "demo",
			path:         "foo/bar.yml",
			filename:     "some-other-name",
			wantDir:      "foo",
			wantFile:     "bar.yml",
		},
		{
			name:         "non-empty filename without yaml suffix on path returns full path and <resource>.yaml",
			resourceName: "demo",
			path:         "foo/bar",
			filename:     "some-other-name",
			wantDir:      "foo/bar",
			wantFile:     "demo.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotDir, gotFile := dt.getFileDirName(tc.resourceName, tc.path, tc.filename)
			if gotDir != tc.wantDir || gotFile != tc.wantFile {
				t.Errorf("got (%q, %q), want (%q, %q)", gotDir, gotFile, tc.wantDir, tc.wantFile)
			}
		})
	}
}
