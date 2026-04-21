package pusher

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGetPathsFromModifiedPaths(t *testing.T) {
	tests := []struct {
		name  string
		input interceptor.ModifiedPaths
		want  []string
	}{
		{
			name:  "both adds and deletes appear, adds first",
			input: interceptor.ModifiedPaths{Add: []string{"a", "b"}, Delete: []string{"x"}},
			want:  []string{"a", "b", "x"},
		},
		{
			name:  "only adds",
			input: interceptor.ModifiedPaths{Add: []string{"a"}},
			want:  []string{"a"},
		},
		{
			name:  "only deletes",
			input: interceptor.ModifiedPaths{Delete: []string{"x"}},
			want:  []string{"x"},
		},
		{
			name:  "both empty",
			input: interceptor.ModifiedPaths{},
			want:  []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetPathsFromModifiedPaths(tc.input)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildCommitMessage(t *testing.T) {
	params := interceptor.GitPipelineParams{
		InterceptedGVR: schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
		InterceptedName: "demo",
	}
	params.RemoteSyncer.Namespace = "default"

	tests := []struct {
		name  string
		paths interceptor.ModifiedPaths
		want  string
	}{
		{
			name:  "adds and deletes both counted",
			paths: interceptor.ModifiedPaths{Add: []string{"a", "b"}, Delete: []string{"x"}},
			want:  "2+1- deployments.apps/v1: default/demo",
		},
		{
			name:  "adds only",
			paths: interceptor.ModifiedPaths{Add: []string{"a"}},
			want:  "1+ deployments.apps/v1: default/demo",
		},
		{
			name:  "deletes only",
			paths: interceptor.ModifiedPaths{Delete: []string{"x"}},
			want:  "1- deployments.apps/v1: default/demo",
		},
		{
			name:  "no paths produces a space prefix",
			paths: interceptor.ModifiedPaths{},
			want:  "deployments.apps/v1: default/demo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildCommitMessage(params, tc.paths); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsErrorSkipable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"ErrEmptyCommit is skipable", git.ErrEmptyCommit, true},
		{"wrapped ErrEmptyCommit is skipable", fmt.Errorf("wrap: %w", git.ErrEmptyCommit), true},
		{"entry not found delete message is skipable", errors.New("failed to delete file in staging area: entry not found"), true},
		{"unrelated error is not skipable", errors.New("some other error"), false},
		{"partial match on delete message does not match", errors.New("entry not found"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isErrorSkipable(tc.err); got != tc.want {
				t.Errorf("got %v, want %v (err: %q)", got, tc.want, tc.err)
			}
		})
	}

	t.Run("substring match is required, not prefix", func(t *testing.T) {
		err := errors.New("prefix: failed to delete file in staging area: entry not found: suffix")
		if !isErrorSkipable(err) {
			t.Errorf("substring anywhere should still match")
		}
		if !strings.Contains(err.Error(), "entry not found") {
			t.Errorf("sanity: test message should contain the expected substring")
		}
	})
}
