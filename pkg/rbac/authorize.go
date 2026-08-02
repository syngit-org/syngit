package rbac

import (
	"context"
	"slices"

	"github.com/syngit-org/syngit/pkg/refs"
	authenticationv1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CheckAccess runs a SubjectAccessReview for the given user against attrs.
func CheckAccess(
	ctx context.Context,
	c client.Client,
	user authenticationv1.UserInfo,
	attrs authv1.ResourceAttributes,
) (bool, error) {
	sar := &authv1.SubjectAccessReview{
		Spec: authv1.SubjectAccessReviewSpec{
			User:               user.Username,
			Groups:             user.Groups,
			UID:                user.UID,
			ResourceAttributes: &attrs,
		},
	}
	if err := c.Create(ctx, sar); err != nil {
		return false, err
	}
	return sar.Status.Allowed, nil
}

// Checks that user is allowed to get every reference that resolves outside of
// ownerNamespace. References resolving into ownerNamespace are not checked at
// all: holding the referencing object already implies access to its own
// namespace, so the common case costs nothing.
//
// Namespaces listed in exempt are skipped as well. This is how the interception
// runtime lets every user reach the operator-owned objects of the manager
// namespace without granting them access to it.
//
// It returns the first reference the user is not allowed to get, or nil when all
// of them are allowed.
func AuthorizeCrossNamespaceRefs(
	ctx context.Context,
	c client.Client,
	user authenticationv1.UserInfo,
	objectRefs []refs.ObjectRef,
	ownerNamespace string,
	exempt ...string,
) (*refs.ObjectRef, error) {
	crossNamespace := make([]refs.ObjectRef, 0, len(objectRefs))
	for _, ref := range objectRefs {
		if ref.Namespace == ownerNamespace || slices.Contains(exempt, ref.Namespace) {
			continue
		}
		crossNamespace = append(crossNamespace, ref)
	}

	return AuthorizeRefs(ctx, c, user, crossNamespace)
}

// Checks that user is allowed to get every one of the given references,
// wherever they resolve. Callers that must not pay for same-namespace
// references use AuthorizeCrossNamespaceRefs instead; callers that gate access
// to the referenced object itself (a RemoteUser's credentials, the RemoteUsers
// a RemoteUserBinding grants) use this one, so that authority over the
// referencing object never implies authority over what it points at.
//
// It returns the first reference the user is not allowed to get, or nil when all
// of them are allowed.
func AuthorizeRefs(
	ctx context.Context,
	c client.Client,
	user authenticationv1.UserInfo,
	objectRefs []refs.ObjectRef,
) (*refs.ObjectRef, error) {
	for _, ref := range objectRefs {
		allowed, err := CheckAccess(ctx, c, user, authv1.ResourceAttributes{
			Namespace: ref.Namespace,
			Verb:      "get",
			Group:     ref.Group,
			Version:   ref.Version,
			Resource:  ref.Resource,
			Name:      ref.Name,
		})
		if err != nil {
			return nil, err
		}
		if !allowed {
			return &ref, nil
		}
	}

	return nil, nil
}
