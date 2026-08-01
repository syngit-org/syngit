package interceptor

import (
	"testing"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"k8s.io/apimachinery/pkg/types"
)

func newTestWebhookInterceptsAll() *WebhookInterceptsAll {
	return &WebhookInterceptsAll{
		pathHandlers: make(map[string]*DynamicWebhookHandler),
	}
}

func TestWebhookPaths(t *testing.T) {
	got := RemoteSyncerWebhookPath(types.NamespacedName{Namespace: "ns", Name: "foo"})
	if got != "/syngit/namespace-scoped-validate/ns/foo" {
		t.Errorf("RemoteSyncerWebhookPath = %q, want /syngit/namespace-scoped-validate/ns/foo", got)
	}

	got = ClusterWideRemoteSyncerWebhookPath("foo")
	if got != "/syngit/cluster-scoped-validate/foo" {
		t.Errorf("ClusterWideRemoteSyncerWebhookPath = %q, want /syngit/cluster-scoped-validate/foo", got)
	}
}

func TestWebhookInterceptsAll_Register(t *testing.T) {
	s := newTestWebhookInterceptsAll()
	rs := syngit.RemoteSyncer{}
	rs.Namespace = "default"
	rs.Name = "my-syncer" // nolint:goconst
	rs.Spec.RemoteRepository = "https://example.com/repo.git"

	path := RemoteSyncerWebhookPath(types.NamespacedName{Namespace: "default", Name: "my-syncer"})
	handler := s.Register(rs, path)

	if handler == nil {
		t.Fatalf("Register returned nil handler")
	}
	stored, ok := s.pathHandlers[path]
	if !ok {
		t.Fatalf("path not registered in pathHandlers map")
	}
	if stored != handler {
		t.Errorf("stored handler differs from returned handler")
	}
	if stored.remoteSyncer.Name != "my-syncer" { // nolint:goconst
		t.Errorf("stored remoteSyncer.Name=%q, want my-syncer", stored.remoteSyncer.Name)
	}

	// Mutating the original RemoteSyncer must not leak into the stored copy.
	rs.Spec.RemoteRepository = "https://mutated/after.git"
	if stored.remoteSyncer.Spec.RemoteRepository == "https://mutated/after.git" {
		t.Errorf("Register should deep-copy the RemoteSyncer; mutation leaked")
	}
}

func TestWebhookInterceptsAll_RegisterClusterWide(t *testing.T) {
	s := newTestWebhookInterceptsAll()
	cwrs := syngit.ClusterWideRemoteSyncer{}
	cwrs.Name = "my-cluster-syncer"
	cwrs.Spec.IdentityStoreNamespace = "identities" // nolint:goconst

	path := ClusterWideRemoteSyncerWebhookPath("my-cluster-syncer")
	handler := s.RegisterClusterWide(cwrs, path)

	stored, ok := s.pathHandlers[path]
	if !ok {
		t.Fatalf("path not registered in pathHandlers map")
	}
	if stored != handler {
		t.Errorf("stored handler differs from returned handler")
	}
	if stored.remoteSyncer != nil {
		t.Errorf("a cluster-wide registration must not populate the namespaced syncer")
	}
	if stored.clusterWideRemoteSyncer.Name != "my-cluster-syncer" {
		t.Errorf("stored name=%q, want my-cluster-syncer", stored.clusterWideRemoteSyncer.Name)
	}
}

// The syncer context is resolved per request, because only the request knows the
// namespace of the object being intercepted.
func TestDynamicWebhookHandler_SyncerContext(t *testing.T) {
	rs := syngit.RemoteSyncer{}
	rs.Namespace = "team-a" // nolint:goconst
	rs.Name = "rsy"
	nsHandler := &DynamicWebhookHandler{remoteSyncer: &rs}

	// A namespaced syncer takes its identity and reference namespaces from
	// itself, and the intercepted namespace from the request.
	sc := nsHandler.syncerContext("team-a")                              // nolint:goconst
	if sc.RUBNamespace != "team-a" || sc.RefOwnerNamespace != "team-a" { // nolint:goconst
		t.Errorf("namespaced context = %+v, want identity and ref namespaces to be team-a", sc)
	}
	if sc.InterceptedNamespace != "team-a" { // nolint:goconst
		t.Errorf("InterceptedNamespace = %q, want team-a", sc.InterceptedNamespace)
	}
	if sc.ClusterWide {
		t.Errorf("namespaced syncer reported as cluster-wide")
	}

	cwrs := syngit.ClusterWideRemoteSyncer{}
	cwrs.Name = "cwrsy"
	cwrs.Spec.IdentityStoreNamespace = "identities" // nolint:goconst
	cwHandler := &DynamicWebhookHandler{clusterWideRemoteSyncer: &cwrs}

	sc = cwHandler.syncerContext("team-b")
	if sc.InterceptedNamespace != "team-b" {
		t.Errorf("InterceptedNamespace = %q, want team-b", sc.InterceptedNamespace)
	}
	if sc.RUBNamespace != "identities" { // nolint:goconst
		t.Errorf("RUBNamespace = %q, want identities", sc.RUBNamespace)
	}
	// No namespace of its own means unqualified references must be rejected.
	if sc.RefOwnerNamespace != "" {
		t.Errorf("RefOwnerNamespace = %q, want empty", sc.RefOwnerNamespace)
	}

	// A cluster-scoped object carries no namespace at all; the path segment
	// substituted for it is the writers' concern, not the context's.
	if sc := cwHandler.syncerContext(""); sc.InterceptedNamespace != "" {
		t.Errorf("InterceptedNamespace = %q, want empty for a cluster-scoped object", sc.InterceptedNamespace)
	}
}

func TestWebhookInterceptsAll_Unregister(t *testing.T) {
	t.Run("removes a previously registered handler", func(t *testing.T) {
		s := newTestWebhookInterceptsAll()
		rs := syngit.RemoteSyncer{}
		rs.Namespace = "ns"
		rs.Name = "foo"

		path := RemoteSyncerWebhookPath(types.NamespacedName{Namespace: "ns", Name: "foo"})
		s.Register(rs, path)
		if _, ok := s.pathHandlers[path]; !ok {
			t.Fatalf("precondition: handler should be registered")
		}

		s.Unregister(path)

		if _, ok := s.pathHandlers[path]; ok {
			t.Errorf("handler should have been removed from map")
		}
	})

	t.Run("unregistering an absent key is a no-op", func(t *testing.T) {
		s := newTestWebhookInterceptsAll()
		// Pre-populate an unrelated handler.
		other := syngit.RemoteSyncer{}
		other.Namespace = "ns"
		other.Name = "other"
		otherPath := RemoteSyncerWebhookPath(types.NamespacedName{Namespace: "ns", Name: "other"})
		s.Register(other, otherPath)

		s.Unregister(RemoteSyncerWebhookPath(types.NamespacedName{Namespace: "ns", Name: "does-not-exist"}))

		if _, ok := s.pathHandlers[otherPath]; !ok {
			t.Errorf("unrelated handler should still be present")
		}
	})
}
