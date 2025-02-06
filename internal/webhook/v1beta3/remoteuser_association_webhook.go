package v1beta3

import (
	"context"
	"net/http"

	patterns "github.com/syngit-org/syngit/internal/patterns/v1beta3"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

/*
	Handle webhook and get kubernetes user id
*/

type RemoteUserAssociationWebhookHandler struct {
	Client  client.Client
	Decoder *admission.Decoder
}

// +kubebuilder:webhook:path=/syngit-v1beta3-remoteuser-association,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remoteusers,verbs=create;update;delete,versions=v1beta3,admissionReviewVersions=v1,name=vremoteusers-association.v1beta3.syngit.io

func (ruwh *RemoteUserAssociationWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	var remoteUser *syngit.RemoteUser
	var isEnabled = false

	if string(req.Operation) == "DELETE" { //nolint:goconst
		remoteUser = &syngit.RemoteUser{}
		err := ruwh.Decoder.DecodeRaw(req.OldObject, remoteUser)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	} else {
		remoteUser = &syngit.RemoteUser{}
		err := ruwh.Decoder.Decode(req, remoteUser)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if remoteUser.Annotations[syngit.RubAnnotation] == "true" {
			isEnabled = true
		}
	}

	username := req.DeepCopy().UserInfo.Username
	associationPattern := &patterns.RemoteUserAssociationPattern{
		PatternSpecification: patterns.PatternSpecification{
			Client:         ruwh.Client,
			NamespacedName: types.NamespacedName{Name: req.Name, Namespace: req.Namespace},
		},
		Username:   username,
		RemoteUser: *remoteUser,
		IsEnabled:  isEnabled,
	}
	remoteTargetPattern := &patterns.RemoteUserOneOrManyBranchPattern{
		PatternSpecification: patterns.PatternSpecification{
			Client:         ruwh.Client,
			NamespacedName: types.NamespacedName{Name: req.Name, Namespace: req.Namespace},
		},
		Username:   username,
		RemoteUser: *remoteUser,
		IsEnabled:  isEnabled,
	}

	err := patterns.Trigger(associationPattern, ctx)
	if err != nil {
		if err.Reason == patterns.Denied {
			return admission.Denied(err.Message)
		}
		if err.Reason == patterns.Errored {
			return admission.Errored(http.StatusInternalServerError, err)
		}
	}

	err = patterns.Trigger(remoteTargetPattern, ctx)
	if err != nil {
		if err.Reason == patterns.Denied {
			return admission.Denied(err.Message)
		}
		if err.Reason == patterns.Errored {
			return admission.Errored(http.StatusInternalServerError, err)
		}
	}

	return admission.Allowed("This object is associated to the " + req.Name + " RemoteUserBinding")
}
