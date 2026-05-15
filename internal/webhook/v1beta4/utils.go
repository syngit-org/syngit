package v1beta4

import (
	"slices"
	"strings"

	authenticationv1 "k8s.io/api/authentication/v1"
)

func doesUserBypassWebhook(user authenticationv1.UserInfo, managerNs string) bool {
	return slices.Contains(user.Groups, "system:masters") || strings.HasPrefix(user.Username, "system:serviceaccount:"+managerNs)
}
