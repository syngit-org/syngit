package features

import (
	"strings"
	"testing"
)

func TestFeatureGates_Enabled(t *testing.T) {
	tests := []struct {
		name  string
		gates FeatureGates
		query Feature
		want  bool
	}{
		{"enabled feature returns true", FeatureGates{ResourceFinder: true}, ResourceFinder, true},
		{"disabled feature returns false", FeatureGates{ResourceFinder: false}, ResourceFinder, false},
		{"unknown feature returns false", FeatureGates{ResourceFinder: true}, Feature("Unknown"), false},
		{"empty gates returns false", FeatureGates{}, ResourceFinder, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.gates.Enabled(tc.query); got != tc.want {
				t.Errorf("Enabled(%q)=%v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestFeatureGates_Set(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		errSubstr string
		check     func(t *testing.T, f FeatureGates)
	}{
		{
			name:  "empty input is no-op",
			input: "",
			check: func(t *testing.T, f FeatureGates) {
				if f[ResourceFinder] != false {
					t.Errorf("default should remain false, got %v", f[ResourceFinder])
				}
			},
		},
		{
			name:  "single valid pair true",
			input: "ResourceFinder=true",
			check: func(t *testing.T, f FeatureGates) {
				if !f[ResourceFinder] {
					t.Errorf("ResourceFinder should be enabled")
				}
			},
		},
		{
			name:  "single valid pair false",
			input: "ResourceFinder=false",
			check: func(t *testing.T, f FeatureGates) {
				if f[ResourceFinder] {
					t.Errorf("ResourceFinder should be disabled")
				}
			},
		},
		{
			name:  "trailing comma tolerated",
			input: "ResourceFinder=true,",
			check: func(t *testing.T, f FeatureGates) {
				if !f[ResourceFinder] {
					t.Errorf("ResourceFinder should be enabled")
				}
			},
		},
		{
			name:  "whitespace around pair tolerated",
			input: "  ResourceFinder = true  ",
			check: func(t *testing.T, f FeatureGates) {
				if !f[ResourceFinder] {
					t.Errorf("ResourceFinder should be enabled")
				}
			},
		},
		{
			name:      "missing equals errors",
			input:     "ResourceFinder",
			wantErr:   true,
			errSubstr: "invalid feature gate format",
		},
		{
			name:      "unknown feature errors",
			input:     "Unknown=true",
			wantErr:   true,
			errSubstr: "unknown feature gate",
		},
		{
			name:      "invalid bool errors",
			input:     "ResourceFinder=notabool",
			wantErr:   true,
			errSubstr: "invalid value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := FeatureGates{ResourceFinder: false}
			err := f.Set(tc.input)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errSubstr)
				}
				if !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, f)
			}
		})
	}
}

func TestFeatureGates_String(t *testing.T) {
	t.Run("empty gates produces empty string", func(t *testing.T) {
		f := FeatureGates{}
		if got := f.String(); got != "" {
			t.Errorf("String()=%q, want empty", got)
		}
	})

	t.Run("single feature formatted as key=value", func(t *testing.T) {
		f := FeatureGates{ResourceFinder: true}
		if got := f.String(); got != "ResourceFinder=true" {
			t.Errorf("String()=%q, want %q", got, "ResourceFinder=true")
		}
	})

	t.Run("multiple features formatted as comma-separated set", func(t *testing.T) {
		f := FeatureGates{
			ResourceFinder:  true,
			Feature("Beta"): false,
		}
		got := f.String()
		// map iteration order is non-deterministic; check contents as a set.
		parts := strings.Split(got, ",")
		if len(parts) != 2 {
			t.Fatalf("String()=%q, want two comma-separated parts", got)
		}
		set := map[string]bool{parts[0]: true, parts[1]: true}
		if !set["ResourceFinder=true"] || !set["Beta=false"] {
			t.Errorf("String()=%q, want both ResourceFinder=true and Beta=false", got)
		}
	})
}
