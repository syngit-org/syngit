package controller

import (
	"context"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	syngit "syngit.io/syngit/api/v1beta1"
)

/*
	Handle webhook and get kubernetes user id
*/

type RemoteUserWebhookHandler struct {
	Client  client.Client
	Decoder *admission.Decoder
}

func (ruwh *RemoteUserWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	username := req.DeepCopy().UserInfo.Username
	name := syngit.RubPrefix + username

	if string(req.Operation) == "DELETE" {
		ru := &syngit.RemoteUser{}
		err := ruwh.Decoder.DecodeRaw(req.OldObject, ru)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		rub := &syngit.RemoteUserBinding{}
		webhookNamespacedName := &types.NamespacedName{
			Name:      name,
			Namespace: req.Namespace,
		}
		rubErr := ruwh.Client.Get(ctx, *webhookNamespacedName, rub)
		if rubErr != nil {
			return admission.Errored(http.StatusBadRequest, rubErr)
		}

		isControlled := false
		ownerRefs := rub.ObjectMeta.DeepCopy().OwnerReferences
		newOwnerRefs := []v1.OwnerReference{}
		for _, owner := range ownerRefs {
			if string(owner.UID) != string(ru.UID) {
				newOwnerRefs = append(newOwnerRefs, owner)
			} else {
				isControlled = true
			}
		}
		rub.ObjectMeta.OwnerReferences = newOwnerRefs

		remoteRefs := rub.Spec.DeepCopy().RemoteRefs
		newRemoteRefs := []corev1.ObjectReference{}
		for _, rm := range remoteRefs {
			if (rm.Name != ru.Name && isControlled) || !isControlled {
				newRemoteRefs = append(newRemoteRefs, rm)
			}
		}
		rub.Spec.RemoteRefs = newRemoteRefs

		if len(newRemoteRefs) != 0 {

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

	ru := &syngit.RemoteUser{}
	err := ruwh.Decoder.Decode(req, ru)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if !ru.Spec.AssociatedRemoteUserBinding {
		return admission.Allowed("This object is not associated with any RemoteUserBinding")
	}

	objRef := corev1.ObjectReference{Name: ru.Name}
	rub := &syngit.RemoteUserBinding{}
	webhookNamespacedName := &types.NamespacedName{
		Name:      name,
		Namespace: req.Namespace,
	}
	err = ruwh.Client.Get(ctx, *webhookNamespacedName, rub)
	if err != nil {
		// The RemoteUserBinding does not exists yet

		// Create the RemoteUserBinding object
		rub.Name = name
		rub.Namespace = req.Namespace

		// ownerRef := v1.OwnerReference{
		// 	Name:       ru.Name,
		// 	APIVersion: ru.APIVersion,
		// 	Kind:       ru.GroupVersionKind().Kind,
		// 	UID:        ru.GetUID(),
		// }
		// ownerRefs := make([]v1.OwnerReference, 0)
		// ownerRefs = append(ownerRefs, ownerRef)
		// rub.ObjectMeta.OwnerReferences = ownerRefs

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

		// Update the list of the RemoteUserBinding object
		// ownerRef := v1.OwnerReference{
		// 	Name:       ru.Name,
		// 	APIVersion: ru.APIVersion,
		// 	Kind:       ru.GroupVersionKind().Kind,
		// 	UID:        ru.GetUID(),
		// }
		// ownerRefs := rub.ObjectMeta.DeepCopy().OwnerReferences
		// ownerRefs = append(ownerRefs, ownerRef)
		// rub.ObjectMeta.OwnerReferences = ownerRefs

		remoteRefs := rub.DeepCopy().Spec.RemoteRefs
		remoteRefs = append(remoteRefs, objRef)
		rub.Spec.RemoteRefs = remoteRefs

		updateErr := ruwh.Client.Update(ctx, rub)
		if updateErr != nil {
			return admission.Errored(http.StatusInternalServerError, updateErr)
		}

	}

	return admission.Allowed("This object is associated to the " + name + " RemoteUserBinding")
}
