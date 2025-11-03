package v1beta3

import (
	"context"
	"fmt"
	"net/http"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	utils "github.com/syngit-org/syngit/pkg/utils"
	authv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

/*
	Handle webhook and get kubernetes user id
*/

type RemoteUserBindingPermissionsWebhookHandler struct {
	Client  client.Client
	Decoder admission.Decoder
}

// +kubebuilder:webhook:path=/syngit-v1beta3-remoteuserbinding-permissions,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remoteuserbindings,verbs=create;update;delete,versions=v1beta3,admissionReviewVersions=v1,name=vremoteuserbindings-permissions.v1beta3.syngit.io

func (rubwh *RemoteUserBindingPermissionsWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	user := req.DeepCopy().UserInfo

	rub := &syngit.RemoteUserBinding{}

	if err := utils.GetObjectFromWebhookRequest(rubwh.Decoder, rub, req); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	namespace := rub.GetNamespace()
	for _, ru := range rub.Spec.RemoteUserRefs {
		if ru.Namespace != "" {
			namespace = ru.Namespace
		}
		sar := &authv1.SubjectAccessReview{
			Spec: authv1.SubjectAccessReviewSpec{
				User:   user.Username,
				Groups: user.Groups,
				ResourceAttributes: &authv1.ResourceAttributes{
					Namespace: namespace,
					Verb:      "get",
					Group:     "syngit.io",
					Version:   "v1beta3",
					Resource:  "remoteusers",
					Name:      ru.Name,
				},
			},
		}

		err := rubwh.Client.Create(context.Background(), sar)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if !sar.Status.Allowed {
			denied := utils.DenyGetRemoteUserError{User: user, RemoteUserRef: ru}
			return admission.Denied(denied.Error())
		}

	}

	return admission.Allowed(fmt.Sprintf("The user %s is allowed to get all the referenced remoteusers.", user))
}
