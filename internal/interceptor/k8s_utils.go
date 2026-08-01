package interceptor

import (
	"context"
	"time"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"github.com/syngit-org/syngit/pkg/utils"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type RemoteSyncerStatusUpdater struct {
	syncer       interceptor.SyncerContext
	group        string
	version      string
	resource     string
	resourceName string
	userInfo     authenticationv1.UserInfo
}

func NewRemoteSyncerStatusUpdater(
	admissionRequest *admissionv1.AdmissionRequest,
	sc interceptor.SyncerContext,
) RemoteSyncerStatusUpdater {
	return RemoteSyncerStatusUpdater{
		syncer:       sc,
		group:        admissionRequest.Resource.Group,
		version:      admissionRequest.Resource.Version,
		resource:     admissionRequest.Resource.Resource,
		resourceName: admissionRequest.Name,
		userInfo:     admissionRequest.UserInfo,
	}
}

func (updater RemoteSyncerStatusUpdater) UpdateRemoteSyncerState(
	ctx context.Context,
	targetRepos []interceptor.GitPushResponse,
	kind syngit.ObservedState,
	lastPushDetails string,
) {
	gvrn := &syngit.JsonGVRN{
		Group:    updater.group,
		Version:  updater.version,
		Resource: updater.resource,
		Name:     updater.resourceName,
	}

	repos := make([]string, 0, len(targetRepos))
	for _, info := range targetRepos {
		repos = append(repos, info.URL)
	}
	commitHashes := make([]string, 0, len(targetRepos))
	for _, info := range targetRepos {
		commitHashes = append(commitHashes, info.CommitHash)
	}

	repoPaths := []string{""}
	if len(targetRepos) > 0 {
		for _, paths := range targetRepos {
			repoPaths = append(repoPaths, paths.Paths...)
		}
	}

	var mutate func(status *syngit.RemoteSyncerStatus)

	switch kind {
	case syngit.LastBypassedObjectStateKey:
		lastBypassedObjectState := syngit.LastBypassedObjectState{
			LastBypassedObjectTime:     v1.Now(),
			LastBypassedObjectUserInfo: updater.userInfo,
			LastBypassedObject:         *gvrn,
		}
		mutate = func(status *syngit.RemoteSyncerStatus) {
			status.LastBypassedObjectState = lastBypassedObjectState
		}
	case syngit.LastObservedObjectStateKey:
		lastObservedObjectState := syngit.LastObservedObjectState{
			LastObservedObjectTime:     v1.Now(),
			LastObservedObjectUsername: updater.userInfo.Username,
			LastObservedObject:         *gvrn,
		}
		mutate = func(status *syngit.RemoteSyncerStatus) {
			status.LastObservedObjectState = lastObservedObjectState
		}
	case syngit.LastPushedObjectStateKey:
		lastPushedObjectState := syngit.LastPushedObjectState{
			LastPushedObjectTime:            v1.Now(),
			LastPushedObject:                *gvrn,
			LastPushedObjectGitPaths:        repoPaths,
			LastPushedObjectGitRepos:        repos,
			LastPushedObjectGitCommitHashes: commitHashes,
			LastPushedGitUser:               updater.userInfo.Username,
			LastPushedObjectStatus:          lastPushDetails,
		}
		mutate = func(status *syngit.RemoteSyncerStatus) {
			status.LastPushedObjectState = lastPushedObjectState
		}
	default:
		return
	}

	updateRemoteSyncerStatus(ctx, updater.syncer, mutate)
}

type RemoteSyncerConditionUpdater struct {
	syncer interceptor.SyncerContext
}

func NewRemoteSyncerConditionUpdater(
	sc interceptor.SyncerContext,
) RemoteSyncerConditionUpdater {
	return RemoteSyncerConditionUpdater{
		syncer: sc,
	}
}

func BuildErrorCondition(details string) v1.Condition {
	return v1.Condition{
		LastTransitionTime: v1.Now(),
		Type:               "Synced",
		Reason:             "WebhookHandlerError",
		Status:             "False",
		Message:            details,
	}
}

func BuildSuccessCondition(details string) v1.Condition {
	return v1.Condition{
		LastTransitionTime: v1.Now(),
		Type:               "Synced",
		Status:             "True",
		Reason:             "WebhookHandlerSucceeded",
		Message:            details,
	}
}

func (updater RemoteSyncerConditionUpdater) UpdateRemoteSyncerConditions(ctx context.Context, condition v1.Condition) {
	updateRemoteSyncerStatus(ctx, updater.syncer, func(status *syngit.RemoteSyncerStatus) {
		status.Conditions = utils.TypeBasedConditionUpdater(status.Conditions, condition)
	})
}

// Applies mutate to the live status of the syncer that
// intercepted the request, whichever kind it is. Both kinds carry a
// RemoteSyncerStatus, so only the fetch and the write differ.
// Only the fields touched by mutate are written: the interception pipeline
// updates the conditions and the observed states in separate calls.
func updateRemoteSyncerStatus(
	ctx context.Context,
	sc interceptor.SyncerContext,
	mutate func(status *syngit.RemoteSyncerStatus),
) {
	_ = log.FromContext(ctx)
	k8sClient := utils.K8sClientFromContext(ctx)

	err := retry.RetryOnConflict(wait.Backoff{
		Steps:    5,
		Duration: 1 * time.Second,
		Factor:   2.0,
		Jitter:   0.1,
	}, func() error {
		if sc.ClusterWide {
			var cwrs syngit.ClusterWideRemoteSyncer
			if err := k8sClient.Get(ctx, sc.Ref, &cwrs); err != nil {
				log.Log.Error(err, "can't get the cluster wide remote syncer "+sc.String())
				return err
			}

			mutate(&cwrs.Status)
			return k8sClient.Status().Update(ctx, &cwrs)
		}

		var rsy syngit.RemoteSyncer
		if err := k8sClient.Get(ctx, sc.Ref, &rsy); err != nil {
			log.Log.Error(err, "can't get the remote syncer "+sc.String())
			return err
		}

		mutate(&rsy.Status)
		return k8sClient.Status().Update(ctx, &rsy)
	})
	if err != nil {
		log.Log.Error(err, "can't update the conditions of the remote syncer "+sc.String())
	}
}
