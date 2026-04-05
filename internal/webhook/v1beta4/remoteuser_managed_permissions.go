package v1beta4

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"slices"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	utils "github.com/syngit-org/syngit/pkg/utils"
	v1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

/*
	Handle webhook and get kubernetes user id
*/

type RemoteUserManagedWebhookHandler struct {
	Client  client.Client
	Decoder admission.Decoder
}

// +kubebuilder:webhook:path=/syngit-v1beta4-remoteuser-managed,mutating=true,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remoteusers,verbs=create;update;delete,versions=v1beta4,admissionReviewVersions=v1,name=mremoteusers-managed.v1beta4.syngit.io,reinvocationPolicy=Never

func (ruwh *RemoteUserManagedWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	user := req.DeepCopy().UserInfo
	ru := &syngit.RemoteUser{}

	if err := utils.GetObjectFromWebhookRequest(ruwh.Decoder, ru, req); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	annotations := ru.GetAnnotations()

	managerNs := os.Getenv("MANAGER_NAMESPACE")

	if req.Operation == v1.Delete {
		// Allow the syngit controller service account and cluster admins
		if isUserAllowed(req.UserInfo, managerNs) {
			return admission.Allowed("System user is allowed to delete any RemoteUser")
		}

		if annotations[syngit.RubAnnotationKeyManaged] == "true" { // nolint:goconst
			// Check ownership from the existing object
			oldRu := &syngit.RemoteUser{}
			if err := ruwh.Decoder.DecodeRaw(req.OldObject, oldRu); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}

			sanitizedUsernameReceived := oldRu.Labels[syngit.K8sUserLabelKey]
			if sanitizedUsernameReceived != utils.Sanitize(user.Username) {
				return admission.Denied("The user is not allowed to delete the RemoteUser of another user")
			}
			return admission.Allowed("The user is allowed to delete its own RemoteUser")
		}
	}

	if req.Operation == v1.Update {
		// Allow the syngit controller service account and cluster admins without re-stamping
		if isUserAllowed(req.UserInfo, managerNs) {
			return admission.Allowed("System user is allowed to update any RemoteUser")
		}

		// Check ownership from the existing object
		oldRu := &syngit.RemoteUser{}
		if err := ruwh.Decoder.DecodeRaw(req.OldObject, oldRu); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if oldRu.Annotations[syngit.RubAnnotationKeyManaged] == "true" {
			sanitizedUsernameReceived := oldRu.Labels[syngit.K8sUserLabelKey]
			if sanitizedUsernameReceived != utils.Sanitize(user.Username) {
				return admission.Denied("The user is not allowed to update the RemoteUser of another user")
			}
		}
	}

	// CREATE operation

	if annotations[syngit.RubAnnotationKeyManaged] == "true" {
		// Stamp (or re-stamp) k8s-user label and annotation
		labels := ru.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[syngit.K8sUserLabelKey] = utils.Sanitize(user.Username)
		ru.SetLabels(labels)

		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[syngit.K8sUserLabelKey] = user.Username
		ru.SetAnnotations(annotations)

		marshaledRu, marshalErr := json.Marshal(ru)
		if marshalErr != nil {
			return admission.Errored(http.StatusInternalServerError, marshalErr)
		}

		return admission.PatchResponseFromRaw(req.Object.Raw, marshaledRu)
	}

	return admission.Allowed("The RemoteUser is not managed")
}

func isUserAllowed(user authenticationv1.UserInfo, managerNs string) bool {
	return slices.Contains(user.Groups, "system:masters") || user.Username == "system:serviceaccount:"+managerNs
}
