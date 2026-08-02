package interceptor

import (
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"k8s.io/apimachinery/pkg/types"
)

// ClusterScopedPathSegment stands in for the namespace segment of the git path
// when the intercepted object is cluster-scoped. "_" is not a legal character in
// a DNS-1123 label, so this can never collide with a real namespace.
const ClusterScopedPathSegment = "_cluster"

// SyncerContext is everything the interception pipeline needs to know about the
// syncer that intercepted a request, resolved for that one request.
//
// It exists because a RemoteSyncer and a ClusterWideRemoteSyncer answer the same
// questions from different places: the namespaced one answers all of them with
// its own .metadata.namespace, the cluster-wide one takes them from its spec and
// from the intercepted object. Below this struct, nothing asks which kind it is.
type SyncerContext struct {
	// The spec shared by both kinds.
	Spec syngit.RemoteSyncerSpec

	// Identity of the syncer object. Namespace is empty for a cluster-wide one.
	Ref types.NamespacedName

	// Annotations of the syncer object. The mutation providers gate on these.
	Annotations map[string]string

	// Whether Ref names a ClusterWideRemoteSyncer. Only the status writer needs
	// this, to know which kind to update.
	ClusterWide bool

	// Namespace of the intercepted object; empty when it is cluster-scoped.
	InterceptedNamespace string

	// Where this syncer's RemoteUserBindings are looked up.
	RUBNamespace string

	// The namespace that unqualified spec references resolve against. Empty for a
	// cluster-wide syncer, which makes refs.ResolveNamespace reject any
	// reference that does not carry its own namespace.
	RefOwnerNamespace string
}

// NewRemoteSyncerContext resolves a namespaced RemoteSyncer. Every namespace it
// needs is its own, which is what makes its behavior identical to before this
// type existed.
func NewRemoteSyncerContext(rs syngit.RemoteSyncer, interceptedNamespace string) SyncerContext {
	return SyncerContext{
		Spec:                 rs.Spec,
		Ref:                  types.NamespacedName{Namespace: rs.Namespace, Name: rs.Name},
		Annotations:          rs.Annotations,
		ClusterWide:          false,
		InterceptedNamespace: interceptedNamespace,
		RUBNamespace:         rs.Namespace,
		RefOwnerNamespace:    rs.Namespace,
	}
}

// NewClusterWideSyncerContext resolves a ClusterWideRemoteSyncer against the
// namespace of the object being intercepted.
func NewClusterWideSyncerContext(cwrs syngit.ClusterWideRemoteSyncer, interceptedNamespace string) SyncerContext {
	return SyncerContext{
		Spec:                 cwrs.Spec.RemoteSyncerSpec,
		Ref:                  types.NamespacedName{Name: cwrs.Name},
		Annotations:          cwrs.Annotations,
		ClusterWide:          true,
		InterceptedNamespace: interceptedNamespace,
		RUBNamespace:         cwrs.Spec.IdentityStoreNamespace,
		RefOwnerNamespace:    "",
	}
}

// String identifies the syncer in logs and commit messages.
func (sc SyncerContext) String() string {
	if sc.ClusterWide {
		return sc.Ref.Name
	}
	return sc.Ref.Namespace + "/" + sc.Ref.Name
}
