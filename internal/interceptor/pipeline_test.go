package interceptor

import (
	"strings"
	"testing"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/interceptor"
)

func TestIsWebhookAllowed(t *testing.T) {
	makeRS := func(strategy syngit.Strategy) syngit.RemoteSyncer {
		rs := syngit.RemoteSyncer{}
		rs.Spec.Strategy = strategy
		return rs
	}

	tests := []struct {
		name     string
		strategy syngit.Strategy
		errored  bool
		want     bool
	}{
		{"CommitApply, no error -> allowed", syngit.CommitApply, false, true},
		{"CommitApply, errored -> denied", syngit.CommitApply, true, false},
		{"CommitOnly, no error -> denied", syngit.CommitOnly, false, false},
		{"CommitOnly, errored -> denied", syngit.CommitOnly, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsWebhookAllowed(makeRS(tc.strategy), tc.errored); got != tc.want {
				t.Errorf("IsWebhookAllowed=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildWebhookSuccessMessage(t *testing.T) {
	t.Run("empty slice produces prefix only", func(t *testing.T) {
		got := BuildWebhookSuccessMessage(nil)
		if !strings.HasPrefix(got, "The resource has been push to:") {
			t.Errorf("message missing prefix: %q", got)
		}
	})

	t.Run("single response with paths and hash appears in message", func(t *testing.T) {
		got := BuildWebhookSuccessMessage([]interceptor.GitPushResponse{
			{URL: "https://git/example/repo.git", Paths: []string{"a.yaml", "b.yaml"}, CommitHash: "abc123"},
		})
		for _, s := range []string{"https://git/example/repo.git", "a.yaml", "b.yaml", "abc123"} {
			if !strings.Contains(got, s) {
				t.Errorf("message %q missing %q", got, s)
			}
		}
	})

	t.Run("multiple responses all appear", func(t *testing.T) {
		got := BuildWebhookSuccessMessage([]interceptor.GitPushResponse{
			{URL: "https://one", Paths: []string{"p1"}, CommitHash: "h1"},
			{URL: "https://two", Paths: []string{"p2"}, CommitHash: "h2"},
		})
		for _, s := range []string{"https://one", "p1", "h1", "https://two", "p2", "h2"} {
			if !strings.Contains(got, s) {
				t.Errorf("message %q missing %q", got, s)
			}
		}
	})
}
