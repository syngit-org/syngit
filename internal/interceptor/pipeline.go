package interceptor

import (
	"context"
	"fmt"
	"net/url"

	"github.com/syngit-org/syngit/internal/pusher"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	se "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/pkg/interceptor"
	admissionv1 "k8s.io/api/admission/v1"
)

func RunInterceptionPipeline(
	ctx context.Context,
	admReq *admissionv1.AdmissionRequest,
	remoteSyncer syngit.RemoteSyncer,
	managerNamespace string,
) admissionv1.AdmissionReview {
	userInfo := admReq.UserInfo

	upstreamRemoteSyncerRepoURL, err := url.Parse(remoteSyncer.Spec.RemoteRepository)
	if err != nil {
		return AdmissionReviewBuilder(
			ctx, se.BuildInterceptorPipelineErr("cannot parse the RemoteSyncer's upstream URL"),
			admReq, false, true, remoteSyncer,
		)
	}

	// Check if is bypass user (SA of argo, flux, etc..)
	isBypassUser, err := IsBypassSubject(userInfo, remoteSyncer)
	if err != nil {
		return AdmissionReviewBuilder(ctx, err.Error(), admReq, false, true, remoteSyncer)
	}
	if isBypassUser {
		return AdmissionReviewBuilder(
			ctx, se.BuildInterceptorPipelineErr("subject bypasses the interception"),
			admReq, true, false, remoteSyncer,
		)
	}

	// Get the intercepted object metadata
	objectMetadata := ExtractObjectMetadataFromAdmissionRequest(admReq)

	// Set the targets using the user credentials
	userRemoteTargets, err := GetUserInfoRemoteTargetsAssociation(
		ctx,
		userInfo,
		upstreamRemoteSyncerRepoURL,
		remoteSyncer,
	)
	if err != nil {
		return AdmissionReviewBuilder(ctx, err.Error(), admReq, false, true, remoteSyncer)
	}

	operation := admReq.Operation
	manifest := ""

	// Convert the request to get the yaml of the object
	if operation != admissionv1.Delete {
		manifest, err = ConvertObjectJSONToYAMLString(
			ctx,
			admReq.Object.Raw,
			managerNamespace,
			remoteSyncer,
		)
		if err != nil {
			return AdmissionReviewBuilder(ctx, se.BuildInterceptorPipelineErr(err.Error()), admReq, false, true, remoteSyncer)
		}
	}

	// Check for deletion
	if len(admReq.Object.Raw) != 0 {
		manifestMap, err := ConvertObjectJSONToYAMLMap(admReq.Object.Raw)
		if err != nil {
			return AdmissionReviewBuilder(ctx, err.Error(), admReq, false, true, remoteSyncer)
		}
		if ContainsDeletionTimestamp(manifestMap) {
			return AdmissionReviewBuilder(
				ctx, se.BuildInterceptorPipelineErr("object is being deleted and the interception already happened"),
				admReq, true, false, remoteSyncer,
			)
		}
	}

	// TLS constructor
	caBundle, err := CABundleBuilder(ctx, remoteSyncer, upstreamRemoteSyncerRepoURL)
	if err != nil {
		return AdmissionReviewBuilder(ctx, se.BuildInterceptorPipelineErr(err.Error()), admReq, false, true, remoteSyncer)
	}

	// Git push
	responses, err := RunGitPushPipeline(GitPushParameters{
		UserInfoRemoteTargets: userRemoteTargets,
		RemoteSyncer:          remoteSyncer,
		YAMLManifest:          manifest,
		ObjectMetadata:        objectMetadata,
		Operation:             operation,
		CABundle:              caBundle,
	})
	if err != nil {
		return AdmissionReviewBuilder(ctx, se.BuildInterceptorPipelineErr(err.Error()), admReq, false, true, remoteSyncer)
	}

	statusUpdater := NewRemoteSyncerStatusUpdater(admReq, remoteSyncer)
	statusUpdater.UpdateRemoteSyncerState(
		ctx, responses, syngit.LastPushedObjectStateKey, "",
	)

	// Check if the webhook is allowed
	if !IsWebhookAllowed(remoteSyncer, false) {
		return AdmissionReviewBuilder(
			ctx, se.BuildInterceptorPipelineErr("the remote syncer is in CommitOnly mode"),
			admReq, false, false, remoteSyncer,
		)
	}

	return AdmissionReviewBuilder(ctx, BuildWebhookSuccessMessage(responses), admReq, true, false, remoteSyncer)
}

type GitPushParameters struct {
	// All the repositories and branches where the
	// modification should be pushed associated to
	// the information of the kubernetes user that
	// has applied or delete the intercepted object.
	UserInfoRemoteTargets map[interceptor.GitUserInfo][]syngit.RemoteTarget

	// The RemoteSyncer that has intercetped the object.
	RemoteSyncer syngit.RemoteSyncer

	// The yaml manifest of the intercepted object.
	YAMLManifest string

	// The metadatas of the intercepted object.
	ObjectMetadata ObjectMetadata

	// The operation that the user made on the intercepted
	// object (CREATE, UPDATE or DELETE).
	Operation admissionv1.Operation

	// Bundle containing the CAs of the targeted git platform(s).
	CABundle []byte
}

func RunGitPushPipeline(params GitPushParameters) ([]interceptor.GitPushResponse, error) {
	responses := make([]interceptor.GitPushResponse, 0, len(params.UserInfoRemoteTargets))

	for userInfo, remoteTargets := range params.UserInfoRemoteTargets {
		for _, remoteTarget := range remoteTargets {
			params := &interceptor.GitPipelineParams{
				RemoteSyncer:    *params.RemoteSyncer.DeepCopy(),
				RemoteTarget:    *remoteTarget.DeepCopy(),
				InterceptedYAML: params.YAMLManifest,
				InterceptedGVR:  params.ObjectMetadata.GVR,
				InterceptedName: params.ObjectMetadata.Name,
				GitUserInfo:     userInfo,
				Operation:       params.Operation,
				CABundle:        params.CABundle,
			}
			res, err := pusher.RunGitPipeline(*params)
			if err != nil {
				return nil, err
			}

			if res.CommitHash == "" {
				return nil, fmt.Errorf("the commit hash is empty")
			}

			responses = append(responses, res)
		}
	}

	return responses, nil
}

// Check if there is no error at all during the pipeline processing
// and if the RemoteSyncer is configured to CommitApply mode.
func IsWebhookAllowed(
	remoteSyncer syngit.RemoteSyncer,
	pipelineErrored bool,
) bool {
	if !pipelineErrored && remoteSyncer.Spec.Strategy == syngit.CommitApply {
		return true
	}
	return false
}

// Build the webhook success message based on the locations
// where the resource has been pushed.
func BuildWebhookSuccessMessage(responses []interceptor.GitPushResponse) string {
	message := "The resource has been push to:\n"
	for _, res := range responses {
		message += fmt.Sprintf("- repo: %s\n  paths:", res.URL)
		for _, path := range res.Paths {
			message += fmt.Sprintf("    %s\n", path)
		}
		message += fmt.Sprintf("  commit hash: %s", res.CommitHash)
	}
	return message
}
