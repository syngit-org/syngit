package v1beta5

import (
	"context"
	"net/http"
	"os"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	utils "github.com/syngit-org/syngit/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ClusterWideRemoteSyncerWebhookHandler struct {
	Client  client.Client
	Decoder admission.Decoder
}

// +kubebuilder:webhook:path=/syngit-v1beta5-clusterwideremotesyncer-rules-permissions,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=clusterwideremotesyncers,verbs=create;update;delete,versions=v1beta5,admissionReviewVersions=v1,name=vclusterwideremotesyncers-rules-permissions.v1beta5.syngit.io

func (cwswh *ClusterWideRemoteSyncerWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	user := req.DeepCopy().UserInfo

	managerNs := os.Getenv("MANAGER_NAMESPACE")
	if doesUserBypassWebhook(user, managerNs) {
		return admission.Allowed("System user is allowed to scope any resources")
	}

	cwrs := &syngit.ClusterWideRemoteSyncer{}

	if err := utils.GetObjectFromWebhookRequest(cwswh.Decoder, cwrs, req); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// The user must be allowed to act on the scoped resources in every namespace
	// this syncer intercepts, not just in one of them.
	namespaces, err := cwswh.selectedNamespaces(ctx, cwrs)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return authorizeSyncer(ctx, cwswh.Client, cwrs, user, namespaces)
}

// Resolves spec.namespaceSelector to the namespaces whose
// permissions must be checked.
//
// A nil or empty selector means "every namespace" (the native behavior of SAR).
func (cwswh *ClusterWideRemoteSyncerWebhookHandler) selectedNamespaces(
	ctx context.Context,
	cwrs *syngit.ClusterWideRemoteSyncer,
) ([]string, error) {
	// An unset selector means every namespace. It has to be handled before
	// LabelSelectorAsSelector, which maps nil to labels.Nothing(): that would
	// select no namespace at all, and a check that reviews zero namespaces
	// authorizes every rule vacuously.
	if cwrs.Spec.NamespaceSelector == nil {
		return []string{""}, nil
	}

	selector, err := metav1.LabelSelectorAsSelector(cwrs.Spec.NamespaceSelector)
	if err != nil {
		return nil, err
	}
	if selector.Empty() {
		return []string{""}, nil
	}

	nsList := &corev1.NamespaceList{}
	if err := cwswh.Client.List(ctx, nsList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}

	namespaces := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	return namespaces, nil
}
