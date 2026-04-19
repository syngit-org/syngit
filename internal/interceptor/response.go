package interceptor

import (
	"context"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/interceptor"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	defaultFailureMessage = "the changes have not been pushed to the remote git repository: "
	defaultSuccessMessage = "the changes were correctly been pushed on the remote git repository"
)

func AdmissionReviewBuilder(
	ctx context.Context,
	addionalMessage string,
	admissionRequest *admissionv1.AdmissionRequest,
	requestAllowed, processErrored bool,
	remoteSyncer syngit.RemoteSyncer,
) admissionv1.AdmissionReview {
	statusUpdater := NewRemoteSyncerStatusUpdater(admissionRequest, remoteSyncer)
	conditionUpdater := NewRemoteSyncerConditionUpdater(remoteSyncer)

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
		conditionUpdater.UpdateRemoteSyncerConditions(ctx, BuildSuccessCondition(""))
	} else {
		conditionUpdater.UpdateRemoteSyncerConditions(ctx, BuildErrorCondition(addionalMessage))
	}

	// Set the final message
	if addionalMessage != "" {
		message += addionalMessage
	}

	statusUpdater.UpdateRemoteSyncerState(
		ctx, []interceptor.GitPushResponse{}, syngit.LastObservedObjectStateKey, message,
	)

	// Construct the admisson review request
	admissionReviewResp := admissionv1.AdmissionReview{
		Response: &admissionv1.AdmissionResponse{
			UID:     admissionRequest.UID,
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
