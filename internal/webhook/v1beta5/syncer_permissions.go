package v1beta5

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/pkg/rbac"
	"github.com/syngit-org/syngit/pkg/refs"
	v1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// authorizeSyncer is the admission gate of both syncer kinds. They carry the
// same RemoteSyncerSpec and therefore ask the same three questions -- may the
// author intercept the scoped resources, borrow the identities, and read the
// referenced objects -- and differ only in the namespaces those questions are
// asked in, which the caller supplies.
//
// namespaces are the namespaces the scoped rules are reviewed against: the
// syncer's own namespace for a RemoteSyncer, the ones selected by
// spec.namespaceSelector for a ClusterWideRemoteSyncer.
func authorizeSyncer(
	ctx context.Context,
	c client.Client,
	syncer syngit.Syncer,
	user v1.UserInfo,
	namespaces []string,
) admission.Response {
	spec := syncer.SyncerSpec()

	if authorized, forbiddenResources, err := hasRightResourcesPermissions(ctx, c, spec.ScopedResources, user, namespaces); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	} else if !authorized {
		return admission.Denied(syngiterrors.NewResourceScopeForbidden(user, forbiddenResources).Error())
	}

	// Pointing a syncer at an identity store means pushing to git under the git
	// credentials of whoever is bound there. A namespaced RemoteSyncer reads its
	// own namespace, which the authority to create it there already covers; a
	// cluster-wide one names whichever namespace it likes, so that has to be
	// earned. Comparing the two namespaces is what tells those cases apart.
	if identityNamespace := syncer.IdentityNamespace(); identityNamespace != syncer.GetNamespace() {
		allowed, err := rbac.CheckAccess(ctx, c, user, authv1.ResourceAttributes{
			Namespace: identityNamespace,
			Verb:      "list",
			Group:     syngit.GroupVersion.Group,
			Version:   syngit.GroupVersion.Version,
			Resource:  "remoteuserbindings",
		})
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		if !allowed {
			return admission.Denied(fmt.Sprintf(
				"the user %s is not allowed to list the remoteuserbindings of the identity store namespace %s",
				user.Username, identityNamespace,
			))
		}
	}

	// The user must be allowed to get every object the syncer references outside
	// of its own namespace. This runs on update as well, so a syncer cannot be
	// edited to point at a namespace the editor cannot read.
	//
	// A cluster-scoped syncer has no own namespace, which makes this stricter in
	// two ways at once: every reference must carry an explicit namespace (there
	// is nothing to default to, and validation rejects the rest), and none of
	// them is exempted as "same namespace".
	ownNamespace := syncer.GetNamespace()
	objectRefs, err := refs.RemoteSyncerRefs(*spec, ownNamespace)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	denied, err := rbac.AuthorizeCrossNamespaceRefs(ctx, c, user, objectRefs, ownNamespace)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if denied != nil {
		return admission.Denied(syngiterrors.NewCrossNamespaceRefDenied(
			user, denied.FieldPath.String(), denied.Resource, denied.Namespace, denied.Name,
		).Error())
	}

	return admission.Allowed(fmt.Sprintf("The user %s is allowed to scope all of the listed resources", user))
}

// Reports whether user may perform every operation of every scoped rule, in
// every one of namespaces. The second return value lists the resources that
// failed, formatted for the denial message.
func hasRightResourcesPermissions(
	ctx context.Context,
	c client.Client,
	scopedResources syngit.ScopedResources,
	user v1.UserInfo,
	namespaces []string,
) (bool, []string, error) {
	forbiddenResourcesMap := map[string]string{}

	for _, rule := range scopedResources.Rules {
		for _, group := range rule.APIGroups {
			for _, version := range rule.APIVersions {
				for _, resource := range rule.Resources {

					forbiddenOperations := []string{}

					for _, operation := range rule.Operations {
						verbs, err := rbac.OperationToVerb(operation)
						if err != nil {
							// Skipping unsupported operation
							continue
						}

						allowed, err := isOperationAllowed(ctx, c, user, namespaces, group, version, resource, verbs)
						if err != nil {
							return false, nil, err
						}
						if !allowed {
							forbiddenOperations = append(forbiddenOperations, string(operation))
						}
					}
					if len(forbiddenOperations) > 0 {
						forbiddenResourcesMap[fmt.Sprintf("%s/%s %s", group, version, resource)] = strings.Join(forbiddenOperations, ", ")
					}
				}
			}
		}
	}

	forbiddenResources := []string{}
	for k, v := range forbiddenResourcesMap {
		forbiddenResources = append(forbiddenResources, fmt.Sprintf("%s [%s]", k, v))
	}

	return len(forbiddenResources) == 0, forbiddenResources, nil
}

// Reports whether user holds at least one of verbs on the resource in every one
// of namespaces. One namespace out of reach forbids the operation outright: the
// syncer would otherwise intercept resources there on behalf of a user who
// cannot touch them.
//
// An empty namespace means "every namespace" to a SubjectAccessReview, so a
// single-element []string{""} asks that question in one call rather than
// enumerating the cluster.
func isOperationAllowed(
	ctx context.Context,
	c client.Client,
	user v1.UserInfo,
	namespaces []string,
	group, version, resource string,
	verbs []string,
) (bool, error) {
	for _, namespace := range namespaces {
		allowedHere := false

		for _, verb := range verbs {
			verbAllowed, err := rbac.CheckAccess(ctx, c, user, authv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      verb,
				Group:     group,
				Version:   version,
				Resource:  resource,
			})
			if err != nil {
				if isInvalidCombinationError(err) {
					// An invalid combination is a property of the group/version/
					// resource, not of the namespace, so it settles the operation
					// everywhere at once. Skipping invalid combination.
					return true, nil
				}
				// For any other error, treat it as critical
				return false, err
			}

			if verbAllowed {
				allowedHere = true
				break
			}
		}

		if !allowedHere {
			return false, nil
		}
	}

	return true, nil
}
