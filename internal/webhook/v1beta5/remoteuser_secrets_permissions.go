package v1beta5

import (
	"context"
	"fmt"
	"net/http"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	utils "github.com/syngit-org/syngit/pkg/utils"
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

	// The user must be allowed to get the referenced Secret, wherever it lives.
	// Its own namespace is checked too: being able to create a RemoteUser must
	// not become a way to use credentials it cannot read.
	refs, err := utils.RemoteUserRefs(ru.Spec, ru.GetNamespace())
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	denied, err := utils.AuthorizeRefs(ctx, ruwh.Client, user, refs)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if denied != nil {
		return denyRef(user, denied, ru.GetNamespace(), syngiterrors.NewCredentialsNotFound(
			fmt.Sprintf("the user %s is not allowed to get the secret for its own remote user", user),
			ru.Spec.SecretRef.Name,
		))
	}

	return admission.Allowed(fmt.Sprintf("The user %s is allowed to get the secret: %s", user, ru.Spec.SecretRef.Name))
}
