package interceptor

import (
	stderrors "errors"
	"testing"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/pkg/interceptor"
	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestIsBypassSubject(t *testing.T) {
	makeRS := func(names ...string) interceptor.SyncerContext {
		rs := syngit.RemoteSyncer{}
		for _, n := range names {
			rs.Spec.BypassInterceptionSubjects = append(rs.Spec.BypassInterceptionSubjects, rbacv1.Subject{Name: n})
		}
		return interceptor.NewRemoteSyncerContext(rs, "")
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

func TestIsServiceAccount(t *testing.T) {
	tests := []struct {
		name     string
		userInfo authenticationv1.UserInfo
		want     bool
	}{
		{
			name:     "regular user",
			userInfo: authenticationv1.UserInfo{Username: "alice"},
			want:     false,
		},
		{
			name: "service account",
			userInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccount:syngit:the-controller",
				Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:syngit"},
			},
			want: true,
		},
		{
			name:     "service account of another namespace",
			userInfo: authenticationv1.UserInfo{Username: "system:serviceaccount:cert-manager:cert-manager"},
			want:     true,
		},
		{
			name:     "empty username",
			userInfo: authenticationv1.UserInfo{},
			want:     false,
		},
		{
			name:     "user impersonating the group but not the username",
			userInfo: authenticationv1.UserInfo{Username: "alice", Groups: []string{"system:serviceaccounts"}},
			want:     false,
		},
		{
			name:     "user named like the prefix without being one",
			userInfo: authenticationv1.UserInfo{Username: "system:serviceaccount"},
			want:     false,
		},
		{
			name:     "prefix in the middle of the username",
			userInfo: authenticationv1.UserInfo{Username: "not-system:serviceaccount:syngit:the-controller"},
			want:     false,
		},
		{
			name:     "system user that is not a service account",
			userInfo: authenticationv1.UserInfo{Username: "system:kube-controller-manager"},
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsServiceAccount(tc.userInfo); got != tc.want {
				t.Errorf("IsServiceAccount(%q)=%v, want %v", tc.userInfo.Username, got, tc.want)
			}
		})
	}
}
