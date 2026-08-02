package interceptor

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/mux"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"github.com/syngit-org/syngit/pkg/kube"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// The webhook paths are built here and nowhere else, so that the path a syncer
// is registered under, the path its webhook entry points the API server at, and
// the path it is unregistered from can never drift apart.
const (
	remoteSyncerPathPrefix = "/syngit/namespace-scoped-validate/"
	clusterWidePathPrefix  = "/syngit/cluster-scoped-validate/"
)

// RemoteSyncerWebhookPath is the interception path of a namespaced RemoteSyncer.
func RemoteSyncerWebhookPath(n types.NamespacedName) string {
	return remoteSyncerPathPrefix + n.Namespace + "/" + n.Name
}

// ClusterWideRemoteSyncerWebhookPath is the interception path of a
// ClusterWideRemoteSyncer. It has no namespace segment.
func ClusterWideRemoteSyncerWebhookPath(name string) string {
	return clusterWidePathPrefix + name
}

type WebhookInterceptsAll struct {
	K8sClient client.Client

	// Caching system
	pathHandlers (map[string]*DynamicWebhookHandler)
	sync.RWMutex

	Manager ctrl.Manager
}

// DynamicWebhookHandler serves one registered syncer. It holds the syncer object
// itself rather than a resolved SyncerContext because the context depends on the
// namespace of the intercepted object, which is only known per request.
type DynamicWebhookHandler struct {
	remoteSyncer            *syngit.RemoteSyncer
	clusterWideRemoteSyncer *syngit.ClusterWideRemoteSyncer
}

// syncerContext resolves the handler's syncer against the object being intercepted.
func (dwc *DynamicWebhookHandler) syncerContext(interceptedNamespace string) interceptor.SyncerContext {
	if dwc.clusterWideRemoteSyncer != nil {
		return interceptor.NewClusterWideSyncerContext(*dwc.clusterWideRemoteSyncer, interceptedNamespace)
	}
	return interceptor.NewRemoteSyncerContext(*dwc.remoteSyncer, interceptedNamespace)
}

// NewWebhookInterceptsAll creates and starts the interception server. There is
// exactly one per manager: it owns a single mux registered at "/syngit/" and a
// single path->handler map, which every syncer controller registers into.
func NewWebhookInterceptsAll(mgr ctrl.Manager) *WebhookInterceptsAll {
	s := &WebhookInterceptsAll{
		K8sClient: mgr.GetClient(),
		Manager:   mgr,
		// Built here rather than in Start so that a syncer registered before the
		// server is started is recorded instead of panicking on a nil map.
		pathHandlers: make(map[string]*DynamicWebhookHandler),
	}
	return s
}

// Start serves the interception routes. It returns the receiver so it can be
// chained onto the constructor.
func (s *WebhookInterceptsAll) Start() *WebhookInterceptsAll {
	ctx := context.Background()
	_ = log.FromContext(ctx)

	ctx = context.WithValue(ctx, kube.ClientCtxKey{}, s.K8sClient)

	s.Lock()
	if s.pathHandlers == nil {
		s.pathHandlers = make(map[string]*DynamicWebhookHandler)
	}
	s.Unlock()

	go func() {
		router := mux.NewRouter()

		// Namespaced RemoteSyncer: the path carries its namespace and name.
		// The route is built from the same constant the clientConfig path is, so
		// the two cannot drift into a silent 404.
		router.HandleFunc(remoteSyncerPathPrefix+"{namespace}/{name}", func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			namespacedName := types.NamespacedName{
				Namespace: vars["namespace"],
				Name:      vars["name"],
			}

			handler, ok := s.handlerFor(r.URL.Path)
			if !ok {
				// Not cached -> search in the k8s api
				found := &syngit.RemoteSyncer{}
				if err := s.K8sClient.Get(context.Background(), namespacedName, found); err != nil {
					http.NotFound(w, r)
					return
				}
				handler = s.Register(*found, r.URL.Path)
			}

			handler.Handle(ctx, w, r)
		}).Methods(http.MethodPost)

		// ClusterWideRemoteSyncer: cluster-scoped, so the path carries only a name.
		router.HandleFunc(clusterWidePathPrefix+"{name}", func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)

			handler, ok := s.handlerFor(r.URL.Path)
			if !ok {
				found := &syngit.ClusterWideRemoteSyncer{}
				if err := s.K8sClient.Get(context.Background(), types.NamespacedName{Name: vars["name"]}, found); err != nil {
					http.NotFound(w, r)
					return
				}
				handler = s.RegisterClusterWide(*found, r.URL.Path)
			}

			handler.Handle(ctx, w, r)
		}).Methods(http.MethodPost)

		// Register the router with the webhook server
		server := s.Manager.GetWebhookServer()
		server.Register("/syngit/", router)
	}()

	return s
}

func (s *WebhookInterceptsAll) handlerFor(path string) (*DynamicWebhookHandler, bool) {
	s.RLock()
	defer s.RUnlock()

	handler, ok := s.pathHandlers[path]
	return handler, ok
}

// Register registers a namespaced RemoteSyncer under path.
func (s *WebhookInterceptsAll) Register(remoteSyncer syngit.RemoteSyncer, path string) *DynamicWebhookHandler {
	return s.register(path, &DynamicWebhookHandler{remoteSyncer: remoteSyncer.DeepCopy()})
}

// RegisterClusterWide registers a ClusterWideRemoteSyncer under path.
func (s *WebhookInterceptsAll) RegisterClusterWide(cwrs syngit.ClusterWideRemoteSyncer, path string) *DynamicWebhookHandler {
	return s.register(path, &DynamicWebhookHandler{clusterWideRemoteSyncer: cwrs.DeepCopy()})
}

func (s *WebhookInterceptsAll) register(path string, handler *DynamicWebhookHandler) *DynamicWebhookHandler {
	s.Lock()
	defer s.Unlock()

	s.pathHandlers[path] = handler

	return handler
}

// Handle processes the incoming dynamic webhook request
func (dwc *DynamicWebhookHandler) Handle(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var admissionReviewReq admissionv1.AdmissionReview
	err := decoder.Decode(&admissionReviewReq)
	if err != nil {
		http.Error(w, "Failed to decode admission review request", http.StatusBadRequest)
		return
	}

	// The namespace of the intercepted object is empty when it is cluster-scoped.
	sc := dwc.syncerContext(admissionReviewReq.Request.Namespace)

	admResponse := RunInterceptionPipeline(ctx, admissionReviewReq.Request, sc, os.Getenv("MANAGER_NAMESPACE"))

	resp, err := json.Marshal(admResponse)
	if err != nil {
		http.Error(w, "Failed to marshal admission review response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(resp)
	if err != nil {
		http.Error(w, "Failed to write admission review response", http.StatusInternalServerError)
		return
	}
}

// Unregister drops the handler registered under path. Callers pass the same path
// they registered, built with one of the *WebhookPath helpers.
func (s *WebhookInterceptsAll) Unregister(path string) {
	s.Lock()
	defer s.Unlock()

	delete(s.pathHandlers, path)
}
