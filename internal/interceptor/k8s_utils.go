package interceptor

import (
	"context"
	"time"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"github.com/syngit-org/syngit/pkg/utils"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type RemoteSyncerStatusUpdater struct {
	remoteSyncer syngit.RemoteSyncer
	group        string
	version      string
	resource     string
	resourceName string
	userInfo     authenticationv1.UserInfo
}

func NewRemoteSyncerStatusUpdater(
	admissionRequest *admissionv1.AdmissionRequest,
	remoteSyncer syngit.RemoteSyncer,
) RemoteSyncerStatusUpdater {
	return RemoteSyncerStatusUpdater{
		remoteSyncer: remoteSyncer,
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

	switch kind {
	case syngit.LastBypassedObjectStateKey:
		lastBypassedObjectState := &syngit.LastBypassedObjectState{
			LastBypassedObjectTime:     v1.Now(),
			LastBypassedObjectUserInfo: updater.userInfo,
			LastBypassedObject:         *gvrn,
		}
		updater.remoteSyncer.Status.LastBypassedObjectState = *lastBypassedObjectState
	case syngit.LastObservedObjectStateKey:
		lastObservedObjectState := &syngit.LastObservedObjectState{
			LastObservedObjectTime:     v1.Now(),
			LastObservedObjectUsername: updater.userInfo.Username,
			LastObservedObject:         *gvrn,
		}
		updater.remoteSyncer.Status.LastObservedObjectState = *lastObservedObjectState
	case syngit.LastPushedObjectStateKey:
		lastPushedObjectState := &syngit.LastPushedObjectState{
			LastPushedObjectTime:            v1.Now(),
			LastPushedObject:                *gvrn,
			LastPushedObjectGitPaths:        repoPaths,
			LastPushedObjectGitRepos:        repos,
			LastPushedObjectGitCommitHashes: commitHashes,
			LastPushedGitUser:               updater.userInfo.Username,
			LastPushedObjectStatus:          lastPushDetails,
		}
		updater.remoteSyncer.Status.LastPushedObjectState = *lastPushedObjectState
	}

	updateRemoteSyncerStatus(ctx, updater.remoteSyncer)
}

type RemoteSyncerConditionUpdater struct {
	remoteSyncer syngit.RemoteSyncer
}

func NewRemoteSyncerConditionUpdater(
	remoteSyncer syngit.RemoteSyncer,
) RemoteSyncerConditionUpdater {
	return RemoteSyncerConditionUpdater{
		remoteSyncer: remoteSyncer,
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
		Reason:             "WebhhokHandlerSucceeded",
		Message:            details,
	}
}

func (updater RemoteSyncerConditionUpdater) UpdateRemoteSyncerConditions(ctx context.Context, condition v1.Condition) {
	conditions := utils.TypeBasedConditionUpdater(updater.remoteSyncer.Status.DeepCopy().Conditions, condition)
	updater.remoteSyncer.Status.Conditions = conditions

	updateRemoteSyncerStatus(ctx, updater.remoteSyncer)
}

func updateRemoteSyncerStatus(
	ctx context.Context,
	remoteSyncer syngit.RemoteSyncer,
) {
	_ = log.FromContext(ctx)
	k8sClient := K8sClientFromContext(ctx)

	namespacedName := types.NamespacedName{
		Namespace: remoteSyncer.Namespace,
		Name:      remoteSyncer.Name,
	}

	err := retry.RetryOnConflict(wait.Backoff{
		Steps:    5,
		Duration: 1 * time.Second,
		Factor:   2.0,
		Jitter:   0.1,
	}, func() error {
		var rsy syngit.RemoteSyncer
		if err := k8sClient.Get(ctx, namespacedName, &rsy); err != nil {
			log.Log.Error(err, "can't get the remote syncer "+remoteSyncer.Namespace+"/"+remoteSyncer.Name)
		}

		rsy.Status = *remoteSyncer.Status.DeepCopy()
		return k8sClient.Status().Update(ctx, &rsy)
	})
	if err != nil {
		log.Log.Error(err, "can't update the conditions of the remote syncer "+remoteSyncer.Namespace+"/"+remoteSyncer.Name)
	}
}

func K8sClientFromContext(ctx context.Context) client.Client {
	return ctx.Value(k8sClientCtxKey{}).(client.Client)
}
