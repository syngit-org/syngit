package v1beta3

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	utils "github.com/syngit-org/syngit/pkg/utils"
	v1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type RemoteSyncerWebhookHandler struct {
	Client  client.Client
	Decoder admission.Decoder
}

// +kubebuilder:webhook:path=/syngit-v1beta3-remotesyncer-rules-permissions,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remotesyncers,verbs=create;update;delete,versions=v1beta3,admissionReviewVersions=v1,name=vremotesyncers-rules-permissions.v1beta3.syngit.io

func (rswh *RemoteSyncerWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	user := req.DeepCopy().UserInfo

	rs := &syngit.RemoteSyncer{}

	if err := utils.GetObjectFromWebhookRequest(rswh.Decoder, rs, req); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if authorized, forbiddenResources, err := rswh.hasRightResourcesPermissions(*rs, user); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	} else {
		if authorized {
			return admission.Allowed(fmt.Sprintf("The user %s is allowed to scope all of the listed resources", user))
		} else {
			denied := utils.ResourceScopeForbiddenError{User: user, ForbiddenResources: forbiddenResources}
			return admission.Denied(denied.Error())
		}
	}
}

func (rswh *RemoteSyncerWebhookHandler) hasRightResourcesPermissions(rs syngit.RemoteSyncer, user v1.UserInfo) (bool, []string, error) {
	forbiddenResourcesMap := map[string]string{}

	for _, rule := range rs.Spec.ScopedResources.Rules {
		for _, group := range rule.APIGroups {
			for _, version := range rule.APIVersions {
				for _, resource := range rule.Resources {

					forbiddenOperations := []string{}

					for _, operation := range rule.Operations {
						verbs, err := utils.OperationToVerb(operation)
						if err != nil {
							// Skipping unsupported operation
							continue
						}
						allowed := false

						for _, verb := range verbs {
							// Create a SubjectAccessReview
							sar := &authv1.SubjectAccessReview{
								Spec: authv1.SubjectAccessReviewSpec{
									User:   user.Username,
									Groups: user.Groups,
									ResourceAttributes: &authv1.ResourceAttributes{
										Namespace: rs.Namespace,
										Verb:      verb,
										Group:     group,
										Version:   version,
										Resource:  resource,
									},
								},
							}
							err := rswh.Client.Create(context.Background(), sar)
							if err != nil {

								if rswh.isInvalidCombinationError(err) {
									// Skipping invalid combination
									allowed = true
									break
								}

								// For any other error, treat it as critical
								return false, nil, err
							}

							if sar.Status.Allowed {
								allowed = true
								break
							}
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

// Handle wrong apiVersion/Kind combination
func (rswh *RemoteSyncerWebhookHandler) isInvalidCombinationError(err error) bool {
	errMsg := err.Error()
	if strings.Contains(errMsg, "no matches for kind") ||
		strings.Contains(errMsg, "could not find the requested resource") {
		return true
	}
	return false
}
