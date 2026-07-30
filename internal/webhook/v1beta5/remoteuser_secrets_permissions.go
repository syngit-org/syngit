package v1beta5

import (
	"context"
	"fmt"
	"net/http"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	utils "github.com/syngit-org/syngit/pkg/utils"
	authv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

/*
	Handle webhook and get kubernetes user id
*/

type RemoteUserPermissionsWebhookHandler struct {
	Client  client.Client
	Decoder admission.Decoder
}

// +kubebuilder:webhook:path=/syngit-v1beta5-remoteuser-permissions,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remoteusers,verbs=create;update;delete,versions=v1beta5,admissionReviewVersions=v1,name=vremoteusers-permissions.v1beta5.syngit.io

func (ruwh *RemoteUserPermissionsWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	user := req.DeepCopy().UserInfo

	ru := &syngit.RemoteUser{}

	if err := utils.GetObjectFromWebhookRequest(ruwh.Decoder, ru, req); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	namespace := ru.GetNamespace()
	if ru.Spec.SecretRef.Namespace != "" {
		namespace = ru.Spec.SecretRef.Namespace
	}

	allowed, err := utils.CheckAccess(ctx, ruwh.Client, user, authv1.ResourceAttributes{
		Namespace: namespace,
		Verb:      "get",
		Group:     "",
		Version:   "v1",
		Resource:  "secrets",
		Name:      ru.Spec.SecretRef.Name,
	})
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if !allowed {
		return admission.Denied(syngiterrors.NewCredentialsNotFound(
			fmt.Sprintf("the user %s is not allowed to get the secret for its own remote user", user),
			ru.Spec.SecretRef.Name,
		).Error())
	}

	return admission.Allowed(fmt.Sprintf("The user %s is allowed to get the secret: %s", user, ru.Spec.SecretRef.Name))
}
