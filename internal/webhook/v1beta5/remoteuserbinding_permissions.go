package v1beta5

import (
	"context"
	"fmt"
	"net/http"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/pkg/rbac"
	"github.com/syngit-org/syngit/pkg/refs"
	"github.com/syngit-org/syngit/pkg/webhooks"
	corev1 "k8s.io/api/core/v1"
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

// +kubebuilder:webhook:path=/syngit-v1beta5-remoteuserbinding-permissions,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remoteuserbindings,verbs=create;update;delete,versions=v1beta5,admissionReviewVersions=v1,name=vremoteuserbindings-permissions.v1beta5.syngit.io

func (rubwh RemoteUserBindingPermissionsWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	user := req.DeepCopy().UserInfo

	rub := &syngit.RemoteUserBinding{}

	if err := webhooks.DecodeObject(rubwh.Decoder, rub, req); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// The user must be allowed to get every referenced RemoteUser and
	// RemoteTarget.
	objectRefs, err := refs.RemoteUserBindingRefs(rub.Spec, rub.GetNamespace())
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	denied, err := rbac.AuthorizeRefs(ctx, rubwh.Client, user, objectRefs)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if denied != nil {
		ref := corev1.ObjectReference{Namespace: denied.Namespace, Name: denied.Name}
		var sameNamespaceErr error = syngiterrors.NewRemoteUserDenied(user, ref)
		if denied.Resource == "remotetargets" {
			sameNamespaceErr = syngiterrors.NewRemoteTargetDenied(user, ref)
		}
		return denyRef(user, denied, rub.GetNamespace(), sameNamespaceErr)
	}

	return admission.Allowed(fmt.Sprintf(
		"The user %s is allowed to get all the referenced remoteusers and remotetargets.", user,
	))
}
