package interceptor

import (
	stderrors "errors"
	"testing"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestIsBypassSubject(t *testing.T) {
	makeRS := func(names ...string) syngit.RemoteSyncer {
		rs := syngit.RemoteSyncer{}
		for _, n := range names {
			rs.Spec.BypassInterceptionSubjects = append(rs.Spec.BypassInterceptionSubjects, rbacv1.Subject{Name: n})
		}
		return rs
	}

	tests := []struct {
		name       string
		user       string
		subjects   []string
		wantBypass bool
		wantErr    bool
	}{
		{"no subjects means no bypass", "alice", nil, false, false},
		{"username matches a subject", "alice", []string{"alice", "bob"}, true, false},
		{"username does not match any subject", "charlie", []string{"alice", "bob"}, false, false},
		{"duplicate matching subjects error with TooMuchSubject", "alice", []string{"alice", "bob", "alice"}, true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bypass, err := IsBypassSubject(authenticationv1.UserInfo{Username: tc.user}, makeRS(tc.subjects...))

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !stderrors.Is(err, syngiterrors.ErrTooMuchSubject) {
					t.Errorf("expected ErrTooMuchSubject, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bypass != tc.wantBypass {
				t.Errorf("bypass=%v, want %v", bypass, tc.wantBypass)
			}
		})
	}
}
