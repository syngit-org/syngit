package v1beta4

import (
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
)

func TestIsUserAllowed(t *testing.T) {
	tests := []struct {
		name             string
		userInfo         authenticationv1.UserInfo
		managerNamespace string
		allowed          bool
	}{
		{
			name: "admin user",
			userInfo: authenticationv1.UserInfo{
				Username: "i-am-admin",
				Groups:   []string{"system:masters", "random-group"},
			},
			managerNamespace: "syngit",
			allowed:          true,
		},
		{
			name: "fake admin user",
			userInfo: authenticationv1.UserInfo{
				Username: "i-am-admin",
				Groups:   []string{"system:admin", "random-group", "admin", "system:", "masters"},
			},
			managerNamespace: "syngit",
			allowed:          false,
		},
		{
			name: "syngit sa syngit ns",
			userInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccount:syngit:the-controller",
				Groups:   []string{"whatever"},
			},
			managerNamespace: "syngit",
			allowed:          true,
		},
		{
			name: "syngit sa syngit other ns",
			userInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccount:syngit-other:",
				Groups:   []string{"whatever"},
			},
			managerNamespace: "syngit-other",
			allowed:          true,
		},
		{
			name: "random service account",
			userInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccount:cert-manager:cert-manager",
				Groups:   []string{"whatever"},
			},
			managerNamespace: "syngit",
			allowed:          false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			allowed := doesUserBypassWebhook(tc.userInfo, tc.managerNamespace)
			if tc.allowed != allowed {
				t.Fatalf("expected tc.allowed to be equal to allowed")
			}
		})
	}
}
