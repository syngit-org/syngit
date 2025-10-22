package v1beta2

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

/*
	Handle webhook and get kubernetes user id
*/

type RemoteUserAssociationWebhookHandler struct {
	Client  client.Client
	Decoder admission.Decoder
}

const (
	managedByLabelKey   = "managed-by"
	managedByLabelValue = "syngit.io"
	k8sUserLabelKey     = "syngit.io/k8s-user"
)

func (ruwh *RemoteUserAssociationWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	username := req.DeepCopy().UserInfo.Username
	name := syngit.RubPrefix + username

	rubs := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			managedByLabelKey: managedByLabelValue,
			k8sUserLabelKey:   username,
		}),
		Namespace: req.Namespace,
	}
	rubErr := ruwh.Client.List(ctx, rubs, listOps)
	if rubErr != nil {
		return admission.Errored(http.StatusInternalServerError, rubErr)
	}

	if len(rubs.Items) > 1 {
		return admission.Denied(fmt.Sprintf("only one RemoteUserBinding for the user %s should be managed by Syngit", username))
	}

	if string(req.Operation) == "DELETE" { //nolint:goconst
		if len(rubs.Items) <= 0 {
			return admission.Allowed("This object was not associated with any RemoteUserBinding")
		} else {
			rub := rubs.Items[0]
			return ruwh.removeRuFromRub(ctx, req, &rub)
		}
	}

	ru := &syngit.RemoteUser{}
	err := ruwh.Decoder.Decode(req, ru)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	objRef := corev1.ObjectReference{Name: ru.Name}

	isAlreadyDefined, rubFoundName, definedErr := ruwh.isAlreadyReferenced(ctx, req.Name, req.Namespace)
	if definedErr != nil {
		return admission.Errored(http.StatusInternalServerError, definedErr)
	}

	if len(rubs.Items) <= 0 {

		if ru.Annotations[syngit.RubAnnotation] == "" || ru.Annotations[syngit.RubAnnotation] == "false" {
			return admission.Allowed("This object is not associated with any RemoteUserBinding")
		}

		// Geneate the RUB object with the right name
		rub, generateErr := ruwh.generateRemoteUserBinding(ctx, name, req.Namespace, 0)
		if generateErr != nil {
			return admission.Errored(http.StatusInternalServerError, generateErr)
		}
		if isAlreadyDefined && name != rubFoundName {
			return admission.Denied(fmt.Sprintf("the RemoteUser is already bound in the RemoteUserBinding %s", rubFoundName))
		}

		// Create the RemoteUserBinding object
		rub.Namespace = req.Namespace

		// Set the labels
		rub.Labels = map[string]string{
			managedByLabelKey: managedByLabelValue,
			k8sUserLabelKey:   username,
		}

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

		rub := rubs.Items[0]
		name = rub.Name

		if isAlreadyDefined && name != rubFoundName {
			return admission.Denied(fmt.Sprintf("the RemoteUser is already bound in the RemoteUserBinding %s", rubFoundName))
		}
		if ru.Annotations[syngit.RubAnnotation] == "" || ru.Annotations[syngit.RubAnnotation] == "false" {
			return ruwh.removeRuFromRub(ctx, req, &rub)
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

		updateErr := ruwh.Client.Update(ctx, &rub)
		if updateErr != nil {
			return admission.Errored(http.StatusInternalServerError, updateErr)
		}

	}

	return admission.Allowed("This object is associated to the " + name + " RemoteUserBinding")
}

func (ruwh *RemoteUserAssociationWebhookHandler) isAlreadyReferenced(ctx context.Context, ruName string, ruNamespace string) (bool, string, error) {
	rubs := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		Namespace: ruNamespace,
	}
	rubErr := ruwh.Client.List(ctx, rubs, listOps)
	if rubErr != nil {
		return false, "", rubErr
	}

	for _, rub := range rubs.Items {
		for _, ru := range rub.Spec.RemoteRefs {
			if ru.Name == ruName {
				if ru.Namespace == "" || (ru.Namespace == ruNamespace) {
					return true, rub.Name, nil
				}
			}
		}
	}

	return false, "", nil
}

func (ruwh *RemoteUserAssociationWebhookHandler) generateRemoteUserBinding(ctx context.Context, name string, namespace string, suffixNumber int) (*syngit.RemoteUserBinding, error) {
	// The RemoteUserBinding does not exists yet
	rub := &syngit.RemoteUserBinding{}

	newName := name
	if suffixNumber > 0 {
		newName = fmt.Sprintf("%s-%d", name, suffixNumber)
	}
	webhookNamespacedName := &types.NamespacedName{
		Name:      newName,
		Namespace: namespace,
	}
	rubErr := ruwh.Client.Get(ctx, *webhookNamespacedName, rub)
	if rubErr == nil {
		return ruwh.generateRemoteUserBinding(ctx, name, namespace, suffixNumber+1)
	} else {
		if strings.Contains(rubErr.Error(), "not found") {
			rub.Name = newName
			rub.Namespace = namespace
			return rub, nil
		}
		return nil, rubErr
	}
}

func (ruwh *RemoteUserAssociationWebhookHandler) removeRuFromRub(ctx context.Context, req admission.Request, rub *syngit.RemoteUserBinding) admission.Response {
	name := rub.DeepCopy().Name

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
