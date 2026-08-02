package interceptor

import (
	"context"
	"fmt"
	"net/url"

	"github.com/syngit-org/syngit/internal/pusher"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	se "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"github.com/syngit-org/syngit/pkg/kube"
	"github.com/syngit-org/syngit/pkg/rbac"
	"github.com/syngit-org/syngit/pkg/refs"
	"github.com/syngit-org/syngit/pkg/render"
	"github.com/syngit-org/syngit/pkg/webhooks"
	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RunInterceptionPipeline(
	ctx context.Context,
	admReq *admissionv1.AdmissionRequest,
	sc interceptor.SyncerContext,
	managerNamespace string,
) admissionv1.AdmissionReview {
	userInfo := admReq.UserInfo

	upstreamRemoteSyncerRepoURL, err := url.Parse(sc.Spec.RemoteRepository)
	if err != nil {
		return AdmissionReviewBuilder(
			ctx, se.BuildInterceptorPipelineErr("cannot parse the RemoteSyncer's upstream URL"),
			admReq, false, true, sc,
		)
	}

	// Check if ServiceAccounts are bypassable
	if sc.Spec.BypassAllServiceAccounts && IsServiceAccount(userInfo) {
		return AdmissionReviewBuilder(
			ctx, se.BuildInterceptorPipelineErr("service account bypasses the interception"),
			admReq, true, false, sc,
		)
	}

	// Check if is bypass user (SA of argo, flux, etc..)
	isBypassUser, err := IsBypassSubject(userInfo, sc)
	if err != nil {
		return AdmissionReviewBuilder(ctx, err.Error(), admReq, false, true, sc)
	}
	if isBypassUser {
		return AdmissionReviewBuilder(
			ctx, se.BuildInterceptorPipelineErr("subject bypasses the interception"),
			admReq, true, false, sc,
		)
	}

	// The author of the intercepted change must be allowed to get every object that
	// the RemoteSyncer references outside of its own namespace. References into the
	// manager namespace are exempt.
	objectRefs, err := refs.RemoteSyncerRefs(sc.Spec, sc.RefOwnerNamespace)
	if err != nil {
		return AdmissionReviewBuilder(ctx, err.Error(), admReq, false, true, sc)
	}
	denied, err := rbac.AuthorizeCrossNamespaceRefs(
		ctx, kube.ClientFromContext(ctx), userInfo, objectRefs, sc.RefOwnerNamespace, managerNamespace,
	)
	if err != nil {
		return AdmissionReviewBuilder(ctx, se.BuildInterceptorPipelineErr(err.Error()), admReq, false, true, sc)
	}
	if denied != nil {
		return AdmissionReviewBuilder(ctx, se.NewCrossNamespaceRefDenied(
			userInfo, denied.FieldPath.String(), denied.Resource, denied.Namespace, denied.Name,
		).Error(), admReq, false, true, sc)
	}

	// Get the intercepted object metadata
	objectMetadata := webhooks.ExtractObjectMetadata(admReq)

	// Set the targets using the user credentials
	userRemoteTargets, err := GetUserInfoRemoteTargetsAssociation(
		ctx,
		userInfo,
		upstreamRemoteSyncerRepoURL,
		sc,
	)
	if err != nil {
		return AdmissionReviewBuilder(ctx, err.Error(), admReq, false, true, sc)
	}

	operation := admReq.Operation
	manifest := ""

	// Convert the request to get the yaml of the object
	if operation != admissionv1.Delete {
		manifest, err = render.ObjectToYAML(
			ctx,
			admReq.Object.Raw,
			managerNamespace,
			sc.Spec,
			sc.RefOwnerNamespace,
		)
		if err != nil {
			return AdmissionReviewBuilder(ctx, se.BuildInterceptorPipelineErr(err.Error()), admReq, false, true, sc)
		}
	}

	// Check for deletion
	if len(admReq.Object.Raw) != 0 {
		manifestMap, err := render.JSONToMap(admReq.Object.Raw)
		if err != nil {
			return AdmissionReviewBuilder(ctx, err.Error(), admReq, false, true, sc)
		}
		if render.ContainsDeletionTimestamp(manifestMap) {
			return AdmissionReviewBuilder(
				ctx, se.BuildInterceptorPipelineErr("object is being deleted and the interception already happened"),
				admReq, true, false, sc,
			)
		}
	}

	// TLS constructor
	caBundle, err := CABundleBuilder(ctx, sc, upstreamRemoteSyncerRepoURL)
	if err != nil {
		return AdmissionReviewBuilder(ctx, se.BuildInterceptorPipelineErr(err.Error()), admReq, false, true, sc)
	}

	// Git push
	responses, err := RunGitPushPipeline(ctx, GitPushParameters{
		UserInfoRemoteTargets: userRemoteTargets,
		Syncer:                sc,
		YAMLManifest:          manifest,
		ObjectMetadata:        objectMetadata,
		Operation:             operation,
		CABundle:              caBundle,
		Cluster:               kube.ClientFromContext(ctx),
	})
	if err != nil {
		if sc.Spec.Strategy == syngit.CommitApply &&
			sc.Spec.DefaultPushErrorBehavior == syngit.Pass {
			return AdmissionReviewBuilder(ctx, se.BuildInterceptorPipelineErr(err.Error()), admReq, true, true, sc)
		}
		return AdmissionReviewBuilder(ctx, se.BuildInterceptorPipelineErr(err.Error()), admReq, false, true, sc)
	}

	statusUpdater := NewRemoteSyncerStatusUpdater(admReq, sc)
	statusUpdater.UpdateRemoteSyncerState(
		ctx, responses, syngit.LastPushedObjectStateKey, "",
	)

	// Check if the webhook is allowed
	if !IsWebhookAllowed(sc, false) {
		return AdmissionReviewBuilder(
			ctx, se.BuildInterceptorPipelineErr("the remote syncer is in CommitOnly mode"),
			admReq, false, false, sc,
		)
	}

	return AdmissionReviewBuilder(ctx, BuildWebhookSuccessMessage(responses), admReq, true, false, sc)
}

type GitPushParameters struct {
	// All the repositories and branches where the
	// modification should be pushed associated to
	// the information of the kubernetes user that
	// has applied or delete the intercepted object.
	UserInfoRemoteTargets map[interceptor.GitUserInfo][]syngit.RemoteTarget

	// The syncer that has intercepted the object, resolved for this request.
	Syncer interceptor.SyncerContext

	// The yaml manifest of the intercepted object.
	YAMLManifest string

	// The metadatas of the intercepted object.
	ObjectMetadata webhooks.ObjectMetadata

	// The operation that the user made on the intercepted
	// object (CREATE, UPDATE or DELETE).
	Operation admissionv1.Operation

	// Bundle containing the CAs of the targeted git platform(s).
	CABundle []byte

	// Cluster reader handed to the mutation providers for live lookups. May be nil.
	Cluster client.Reader
}

func RunGitPushPipeline(ctx context.Context, params GitPushParameters) ([]interceptor.GitPushResponse, error) {
	responses := make([]interceptor.GitPushResponse, 0, len(params.UserInfoRemoteTargets))

	cluster := params.Cluster

	for userInfo, remoteTargets := range params.UserInfoRemoteTargets {
		for _, remoteTarget := range remoteTargets {
			params := &interceptor.GitPipelineParams{
				Syncer:          params.Syncer,
				RemoteTarget:    *remoteTarget.DeepCopy(),
				InterceptedYAML: params.YAMLManifest,
				InterceptedGVR:  params.ObjectMetadata.GVR,
				InterceptedName: params.ObjectMetadata.Name,
				GitUserInfo:     userInfo,
				Operation:       params.Operation,
				CABundle:        params.CABundle,
			}
			res, err := pusher.RunGitPipeline(ctx, cluster, *params)
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
	sc interceptor.SyncerContext,
	pipelineErrored bool,
) bool {
	if !pipelineErrored && sc.Spec.Strategy == syngit.CommitApply {
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
