package v1beta5

import (
	"context"
	"net/http"
	"os"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	utils "github.com/syngit-org/syngit/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type RemoteSyncerWebhookHandler struct {
	Client  client.Client
	Decoder admission.Decoder
}

// +kubebuilder:webhook:path=/syngit-v1beta5-remotesyncer-rules-permissions,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remotesyncers,verbs=create;update;delete,versions=v1beta5,admissionReviewVersions=v1,name=vremotesyncers-rules-permissions.v1beta5.syngit.io

func (rswh *RemoteSyncerWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	user := req.DeepCopy().UserInfo

	managerNs := os.Getenv("MANAGER_NAMESPACE")
	if doesUserBypassWebhook(user, managerNs) {
		return admission.Allowed("System user is allowed to scope any resources")
	}

	rs := &syngit.RemoteSyncer{}

	if err := utils.GetObjectFromWebhookRequest(rswh.Decoder, rs, req); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// A namespaced RemoteSyncer only ever intercepts its own namespace, so that
	// is the single namespace its rules are reviewed against.
	return authorizeSyncer(ctx, rswh.Client, rs, user, []string{rs.Namespace})
}
