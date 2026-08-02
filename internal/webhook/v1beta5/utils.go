package v1beta5

import (
	"slices"
	"strings"

	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/pkg/refs"
	authenticationv1 "k8s.io/api/authentication/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func doesUserBypassWebhook(user authenticationv1.UserInfo, managerNs string) bool {
	return slices.Contains(user.Groups, "system:masters") || strings.HasPrefix(user.Username, "system:serviceaccount:"+managerNs)
}

// Turns a reference the user may not get into an admission denial.
func denyRef(
	user authenticationv1.UserInfo,
	denied *refs.ObjectRef,
	ownerNamespace string,
	sameNamespaceErr error,
) admission.Response {
	if denied.Namespace != ownerNamespace {
		return admission.Denied(syngiterrors.NewCrossNamespaceRefDenied(
			user, denied.FieldPath.String(), denied.Resource, denied.Namespace, denied.Name,
		).Error())
	}
	return admission.Denied(sameNamespaceErr.Error())
}

// Handle wrong apiVersion/Kind combination
func isInvalidCombinationError(err error) bool {
	errMsg := err.Error()
	if strings.Contains(errMsg, "no matches for kind") ||
		strings.Contains(errMsg, "could not find the requested resource") {
		return true
	}
	return false
}
