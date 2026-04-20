package interceptor

import (
	"testing"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"k8s.io/apimachinery/pkg/types"
)

func newTestWebhookInterceptsAll() *WebhookInterceptsAll {
	return &WebhookInterceptsAll{
		pathHandlers: make(map[string]*DynamicWebhookHandler),
	}
}

func TestWebhookInterceptsAll_Register(t *testing.T) {
	s := newTestWebhookInterceptsAll()
	rs := syngit.RemoteSyncer{}
	rs.Namespace = "default"
	rs.Name = "my-syncer" // nolint:goconst
	rs.Spec.RemoteRepository = "https://example.com/repo.git"

	handler := s.Register(rs, "/syngit/validate/default/my-syncer")

	if handler == nil {
		t.Fatalf("Register returned nil handler")
	}
	stored, ok := s.pathHandlers["/syngit/validate/default/my-syncer"]
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

func TestWebhookInterceptsAll_Unregister(t *testing.T) {
	t.Run("removes a previously registered handler", func(t *testing.T) {
		s := newTestWebhookInterceptsAll()
		rs := syngit.RemoteSyncer{}
		rs.Namespace = "ns"
		rs.Name = "foo"

		path := "/syngit/validatens/foo"
		s.Register(rs, path)
		if _, ok := s.pathHandlers[path]; !ok {
			t.Fatalf("precondition: handler should be registered")
		}

		s.Unregister(types.NamespacedName{Namespace: "ns", Name: "foo"})

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
		s.Register(other, "/syngit/validatens/other")

		s.Unregister(types.NamespacedName{Namespace: "ns", Name: "does-not-exist"})

		if _, ok := s.pathHandlers["/syngit/validatens/other"]; !ok {
			t.Errorf("unrelated handler should still be present")
		}
	})
}
