package interceptor

import (
	"context"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	defaultFailureMessage = "the changes have not been pushed to the remote git repository: "
	defaultSuccessMessage = "the changes were correctly been pushed on the remote git repository."
)

func AdmissionReviewBuilder(
	ctx context.Context,
	addionalMessage string,
	admissionRequestUID types.UID,
	requestAllowed, processErrored bool,
	remoteSyncer syngit.RemoteSyncer,
) admissionv1.AdmissionReview {

	successMessage := defaultSuccessMessage
	if remoteSyncer.Spec.DefaultBlockAppliedMessage != "" {
		successMessage = remoteSyncer.Spec.DefaultBlockAppliedMessage
	}

	// Set the status and the message depending of the status of the webhook
	status := "Failure"
	message := defaultFailureMessage
	if !processErrored {
		status = "Success"
		message = successMessage
	} else {
		addionalMessage = syngiterrors.NewInterceptorPipeline(addionalMessage).Error()
		condition := &v1.Condition{
			LastTransitionTime: v1.Now(),
			Type:               "Synced",
			Reason:             "WebhookHandlerError",
			Status:             "False",
			Message:            addionalMessage,
		}
		updater := NewRemoteSyncerConditionUpdater(remoteSyncer)
		updater.UpdateRemoteSyncerConditions(ctx, *condition)
	}

	// Set the final message
	if addionalMessage != "" {
		message += addionalMessage
	}

	// Construct the admisson review request
	admissionReviewResp := admissionv1.AdmissionReview{
		Response: &admissionv1.AdmissionResponse{
			UID:     admissionRequestUID,
			Allowed: requestAllowed,
			Result: &v1.Status{
				Status:  status,
				Message: message,
			},
		},
	}
	admissionReviewResp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "admission.k8s.io",
		Version: "v1",
		Kind:    "AdmissionReview",
	})

	return admissionReviewResp
}
