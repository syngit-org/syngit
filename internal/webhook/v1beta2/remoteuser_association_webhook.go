package v1beta2

import (
	"context"
	"net/http"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

/*
	Handle webhook and get kubernetes user id
*/

type RemoteUserWebhookHandler struct {
	Client  client.Client
	Decoder *admission.Decoder
}

// +kubebuilder:webhook:path=/syngit-v1beta2-remoteuser-association,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remoteusers,verbs=create;update;delete,versions=v1beta2,admissionReviewVersions=v1,name=vremoteusers-association.v1beta2.syngit.io

func (ruwh *RemoteUserWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	username := req.DeepCopy().UserInfo.Username
	name := syngit.RubPrefix + username

	rub := &syngit.RemoteUserBinding{}
	webhookNamespacedName := &types.NamespacedName{
		Name:      name,
		Namespace: req.Namespace,
	}
	rubErr := ruwh.Client.Get(ctx, *webhookNamespacedName, rub)

	if string(req.Operation) == "DELETE" {
		if rubErr != nil {
			return admission.Allowed("This object was not associated with any RemoteUserBinding")
		} else {
			return ruwh.removeRuFromRub(ctx, req, name, rub)
		}
	}

	ru := &syngit.RemoteUser{}
	err := ruwh.Decoder.Decode(req, ru)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	objRef := corev1.ObjectReference{Name: ru.Name}

	if rubErr != nil {
		// The RemoteUserBinding does not exists yet
		if ru.Annotations["syngit.io/associated-remote-userbinding"] == "" || ru.Annotations["syngit.io/associated-remote-userbinding"] == "false" {
			return admission.Allowed("This object is not associated with any RemoteUserBinding")
		}

		// Create the RemoteUserBinding object
		rub.Name = name
		rub.Namespace = req.Namespace

		subject := &rbacv1.Subject{
			Kind: "User",
			Name: username,
		}
		rub.Spec.Subject = *subject

		remoteRefs := make([]corev1.ObjectReference, 0)
		remoteRefs = append(remoteRefs, objRef)
		rub.Spec.RemoteRefs = remoteRefs

		createErr := ruwh.Client.Create(ctx, rub)
		if createErr != nil {
			return admission.Errored(http.StatusInternalServerError, createErr)
		}
	} else {
		// The RemoteUserBinding already exists
		if ru.Annotations["syngit.io/associated-remote-userbinding"] == "" || ru.Annotations["syngit.io/associated-remote-userbinding"] == "false" {
			return ruwh.removeRuFromRub(ctx, req, name, rub)
		}

		dontAppend := false
		remoteRefs := rub.DeepCopy().Spec.RemoteRefs
		for _, ruRef := range remoteRefs {
			if ruRef.Name == ru.Name {
				dontAppend = true
			}
		}
		if !dontAppend {
			remoteRefs = append(remoteRefs, objRef)
		}
		rub.Spec.RemoteRefs = remoteRefs

		updateErr := ruwh.Client.Update(ctx, rub)
		if updateErr != nil {
			return admission.Errored(http.StatusInternalServerError, updateErr)
		}

	}

	return admission.Allowed("This object is associated to the " + name + " RemoteUserBinding")
}

func (ruwh *RemoteUserWebhookHandler) removeRuFromRub(ctx context.Context, req admission.Request, name string, rub *syngit.RemoteUserBinding) admission.Response {
	ru := &syngit.RemoteUser{}
	err := ruwh.Decoder.DecodeRaw(req.OldObject, ru)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	remoteRefs := rub.Spec.DeepCopy().RemoteRefs
	newRemoteRefs := []corev1.ObjectReference{}
	for _, rm := range remoteRefs {
		if rm.Name != ru.Name {
			newRemoteRefs = append(newRemoteRefs, rm)
		}
	}

	if len(newRemoteRefs) != 0 {

		rub.Spec.RemoteRefs = newRemoteRefs
		deleteErr := ruwh.Client.Update(ctx, rub)
		if deleteErr != nil {
			return admission.Errored(http.StatusInternalServerError, deleteErr)
		}

	} else {

		deleteErr := ruwh.Client.Delete(ctx, rub)
		if deleteErr != nil {
			return admission.Errored(http.StatusInternalServerError, deleteErr)
		}
	}

	return admission.Allowed("This object is not associated with the " + name + " RemoteUserBinding anymore")
}
