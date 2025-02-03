package v1beta3

import (
	"context"
	"net/http"

	patterns "github.com/syngit-org/syngit/internal/patterns/v1beta3"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	utils "github.com/syngit-org/syngit/pkg/utils"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type RemoteSyncerTargetPatternWebhookHandler struct {
	Client  client.Client
	Decoder *admission.Decoder
}

// +kubebuilder:webhook:path=/syngit-v1beta3-remotesyncer-target-pattern,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remotesyncers,verbs=create;update;delete,versions=v1beta3,admissionReviewVersions=v1,name=vremotesyncers-target-pattern.v1beta3.syngit.io

func (rsyt *RemoteSyncerTargetPatternWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	var remoteSyncer *syngit.RemoteSyncer
	var oldBranches = []string{}
	var newBranches = []string{}

	if string(req.Operation) != "CREATE" { //nolint:goconst
		remoteSyncer = &syngit.RemoteSyncer{}
		err := rsyt.Decoder.DecodeRaw(req.OldObject, remoteSyncer)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		oldBranches = utils.GetBranchesFromAnnotation(remoteSyncer.Annotations[syngit.RtAnnotationBranches])
	}

	if string(req.Operation) != "DELETE" { //nolint:goconst
		remoteSyncer = &syngit.RemoteSyncer{}
		err := rsyt.Decoder.Decode(req, remoteSyncer)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		newBranches = utils.GetBranchesFromAnnotation(remoteSyncer.Annotations[syngit.RtAnnotationBranches])
	}

	var pattern patterns.Pattern

	username := req.DeepCopy().UserInfo.Username
	pattern = &patterns.RemoteSyncerOneOrManyBranchPattern{
		PatternSpecification: patterns.PatternSpecification{
			Client:         rsyt.Client,
			Username:       username,
			NamespacedName: types.NamespacedName{Name: req.Name, Namespace: req.Namespace},
		},
		UpstreamRepo:          remoteSyncer.Spec.RemoteRepository,
		UpstreamBranch:        remoteSyncer.Spec.DefaultBranch,
		TargetRepository:      remoteSyncer.Spec.RemoteRepository,
		TargetBranches:        newBranches,
		GarbageTargetBranches: oldBranches,
	}

	err := pattern.Trigger(ctx)
	if err != nil {
		if err.Reason == patterns.Denied {
			return admission.Denied(err.Message)
		}
		if err.Reason == patterns.Errored {
			return admission.Errored(http.StatusInternalServerError, err)
		}
	}

	return admission.Allowed("No differences concerning RemoteTargets")
}
