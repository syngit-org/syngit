package interceptor

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type WebhookInterceptsAll struct {
	K8sClient client.Client

	// Caching system
	pathHandlers (map[string]*DynamicWebhookHandler)
	sync.RWMutex

	Manager ctrl.Manager
}

// PathHandler represents an instance of a path handler with a specific namespace and name
type DynamicWebhookHandler struct {
	remoteSyncer syngit.RemoteSyncer
	k8sClient    client.Client
}

func (s *WebhookInterceptsAll) Start() {
	ctx := context.Background()
	_ = log.FromContext(ctx)

	s.pathHandlers = make(map[string]*DynamicWebhookHandler)

	go func() {
		router := mux.NewRouter()

		// Register a route with placeholders for namespace and name
		router.HandleFunc("/syngit/validate/{namespace}/{name}", func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			namespace := vars["namespace"]
			name := vars["name"]

			// Get the path from the request URL
			path := r.URL.Path

			// Find the appropriate path handler based on the request path
			s.RLock()
			handler, ok := s.pathHandlers[path]
			s.RUnlock()

			// If a handler is found, invoke it
			if ok {
				handler.Handle(w, r)
				return

				// If not, then it is not cached -> search in k8s api
			} else {
				ctx := context.Background()
				namespacedName := &types.NamespacedName{
					Namespace: namespace,
					Name:      name,
				}

				found := &syngit.RemoteSyncer{}
				err := s.K8sClient.Get(ctx, *namespacedName, found)
				if err != nil {
					// If no handler is found, respond with a 404 Not Found status
					http.NotFound(w, r)
					return
				}

				// If found in k8s api, add it to the cached map and handle the request
				handler := s.Register(*found, path)
				handler.Handle(w, r)
			}

		}).Methods(http.MethodPost) // Limit to POST requests if it's a webhook

		// Register the router with the webhook server
		server := s.Manager.GetWebhookServer()
		server.Register("/syngit/", router)
	}()
}

// Register registers the path in the pathHandlers map
func (s *WebhookInterceptsAll) Register(interceptor syngit.RemoteSyncer, path string) *DynamicWebhookHandler {
	s.Lock()
	defer s.Unlock()

	// Create a new path handler with the specified namespace and name
	handler := &DynamicWebhookHandler{
		remoteSyncer: *interceptor.DeepCopy(),
		k8sClient:    s.K8sClient,
	}

	// Register the path handler with the server
	s.pathHandlers[path] = handler

	return handler
}

// Handle processes the incoming dynamic webhook request
func (dwc *DynamicWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var admissionReviewReq admissionv1.AdmissionReview
	err := decoder.Decode(&admissionReviewReq)
	if err != nil {
		http.Error(w, "Failed to decode admission review request", http.StatusBadRequest)
		return
	}

	wrc := &WebhookRequestChecker{
		admReview:    admissionReviewReq,
		remoteSyncer: dwc.remoteSyncer,
		k8sClient:    dwc.k8sClient,
	}

	admResponse := wrc.ProcessSteps()

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

// Unregister removes the specific webhook from the pathHandlers map
func (s *WebhookInterceptsAll) Unregister(n types.NamespacedName) {
	s.Lock()
	defer s.Unlock()

	path := "/syngit/validate" + n.Namespace + "/" + n.Name

	// Unregister the path handler from the server
	delete(s.pathHandlers, path)
}
