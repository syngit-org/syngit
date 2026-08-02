package policy

import (
	"context"
	"reflect"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Returns every syncer other than self, of either kind, that carries a
// non-empty annotationKey and draws its policy-managed RemoteTargets
// from the same identity namespace as self.
//
// Both kinds have to be listed: a namespaced RemoteSyncer and a
// ClusterWideRemoteSyncer can now share one identity namespace,
// so a policy pruning targets on behalf of one must not delete
// targets the other still uses.
func getOtherSyncersWith(
	ctx context.Context,
	c client.Client,
	self syngit.Syncer,
	annotationKey string,
) ([]syngit.Syncer, error) {
	identityNamespace := self.IdentityNamespace()
	var others []syngit.Syncer

	rsList := &syngit.RemoteSyncerList{}
	if err := c.List(ctx, rsList, &client.ListOptions{Namespace: identityNamespace}); err != nil {
		return nil, err
	}
	for i := range rsList.Items {
		rs := &rsList.Items[i]
		if isSameSyncer(rs, self) || rs.Annotations[annotationKey] == "" {
			continue
		}
		others = append(others, rs)
	}

	// Cluster-wide syncers are not namespaced, so they are filtered on the
	// identity namespace they name rather than by the List itself.
	cwrsList := &syngit.ClusterWideRemoteSyncerList{}
	if err := c.List(ctx, cwrsList); err != nil {
		return nil, err
	}
	for i := range cwrsList.Items {
		cwrs := &cwrsList.Items[i]
		if cwrs.Spec.IdentityStoreNamespace != identityNamespace {
			continue
		}
		if isSameSyncer(cwrs, self) || cwrs.Annotations[annotationKey] == "" {
			continue
		}
		others = append(others, cwrs)
	}

	return others, nil
}

// Reports whether a and b are the same object. The kind has to be
// compared as well: a RemoteSyncer and a ClusterWideRemoteSyncer
// may share a name.
func isSameSyncer(a, b syngit.Syncer) bool {
	return reflect.TypeOf(a) == reflect.TypeOf(b) &&
		client.ObjectKeyFromObject(a) == client.ObjectKeyFromObject(b)
}
