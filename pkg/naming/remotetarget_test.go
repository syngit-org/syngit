package naming

import (
	"strings"
	"testing"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
)

func TestRemoteTargetName(t *testing.T) {
	tests := []struct {
		name           string
		upstreamRepo   string
		upstreamBranch string
		targetRepo     string
		targetBranch   string
		wantErr        bool
		wantContains   []string
		wantExact      string
	}{
		{
			name:           "typical URLs produce expected name",
			upstreamRepo:   "https://github.com/org/upstream.git",
			upstreamBranch: "main",
			targetRepo:     "https://github.com/org/target.git",
			targetBranch:   "feature",
			wantContains:   []string{"org-upstream", "main", "org-target", "feature"},
		},
		{
			name:           "empty target uses default fork prefix",
			upstreamRepo:   "https://github.com/org/upstream.git",
			upstreamBranch: "main",
			targetRepo:     "",
			targetBranch:   "main",
			wantContains:   []string{syngit.RtManagedDefaultForkNamePrefix},
		},
		{
			name:           "branches are lowercased",
			upstreamRepo:   "https://github.com/org/upstream.git",
			upstreamBranch: "MAIN",
			targetRepo:     "https://github.com/org/target.git",
			targetBranch:   "FEATURE",
			wantContains:   []string{"-main", "-feature"},
		},
		{
			name:           ".git suffix is stripped from repo names",
			upstreamRepo:   "https://github.com/org/upstream.git",
			upstreamBranch: "main",
			targetRepo:     "https://github.com/org/target.git",
			targetBranch:   "main",
			// "target.git" should not appear (stripped); "target" should.
			wantContains: []string{"target"},
		},
		{
			name:           "invalid URL returns error",
			upstreamRepo:   "://not-a-url",
			upstreamBranch: "main",
			targetRepo:     "",
			targetBranch:   "main",
			wantErr:        true,
		},
		{
			name:           "invalid target URL returns error",
			upstreamRepo:   "https://github.com/org/upstream.git",
			upstreamBranch: "main",
			targetRepo:     "://bad-target",
			targetBranch:   "main",
			wantErr:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RemoteTargetName(tc.upstreamRepo, tc.upstreamBranch, tc.targetRepo, tc.targetBranch)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got name=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantExact != "" && got != tc.wantExact {
				t.Errorf("name=%q, want %q", got, tc.wantExact)
			}
			for _, sub := range tc.wantContains {
				if !strings.Contains(got, sub) {
					t.Errorf("name=%q should contain %q", got, sub)
				}
			}
			if strings.Contains(got, ".git") {
				t.Errorf("name=%q should not contain .git suffix", got)
			}
		})
	}
}
