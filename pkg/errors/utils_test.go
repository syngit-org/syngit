package errors

import (
	stderrors "errors"
	"testing"
)

func TestIs(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target SyngitError
		want   bool
	}{
		{
			name:   "matches wrapped sentinel",
			err:    NewRemoteUserNotFound("details"),
			target: ErrRemoteUserNotFound,
			want:   true,
		},
		{
			name:   "matches by substring from different constructor of same type",
			err:    NewGitPipeline("boom"),
			target: ErrGitPipeline,
			want:   true,
		},
		{
			name:   "does not match unrelated error",
			err:    stderrors.New("unrelated"),
			target: ErrRemoteUserNotFound,
			want:   false,
		},
		{
			name:   "does not cross-match different syngit errors",
			err:    NewWrongLabelParsing("bad"),
			target: ErrRemoteTargetNotFound,
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Is(tc.err, tc.target); got != tc.want {
				t.Errorf("Is()=%v, want %v", got, tc.want)
			}
		})
	}
}
