package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	syngit "syngit.io/syngit/api/v1beta1"
)

type WebhookInterceptsAll struct {
	server    *http.Server
	stopped   chan struct{}
	log       *logr.Logger
	k8sClient client.Client

	// Caching system
	pathHandlers (map[string]*DynamicWebhookHandler)
	sync.RWMutex

	dev bool
}

// PathHandler represents an instance of a path handler with a specific namespace and name
type DynamicWebhookHandler struct {
	remoteSyncer syngit.RemoteSyncer
	k8sClient    client.Client
	log          *logr.Logger
}

// Start starts the webhook server
func (s *WebhookInterceptsAll) Start() {
	var log = logf.Log.WithName("remotesyncer-webhook")
	s.log = &log

	s.Lock()
	defer s.Unlock()

	if s.server != nil {
		return
	}

	s.pathHandlers = make(map[string]*DynamicWebhookHandler)
	s.stopped = make(chan struct{})

	// Create the HTTP server
	s.server = &http.Server{
		Addr: ":9444",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get the path from the request URL
			path := r.URL.Path

			// Find the appropriate path handler based on the request path
			s.RLock()
			handler, ok := s.pathHandlers[path]
			s.RUnlock()

			// If a handler is found, invoke it
			if ok {
				handler.ServeHTTP(w, r)
				return

				// If not, then it is not cached -> search in k8s api
			} else {
				pathArray := strings.Split(path, "/")
				riNamespace := pathArray[len(pathArray)-2]
				riName := pathArray[len(pathArray)-1]
				ctx := context.Background()
				riNamespacedName := &types.NamespacedName{
					Namespace: riNamespace,
					Name:      riName,
				}

				found := &syngit.RemoteSyncer{}
				err := s.k8sClient.Get(ctx, *riNamespacedName, found)
				if err != nil {
					// If no handler is found, respond with a 404 Not Found status
					http.NotFound(w, r)
					return
				}

				// If found in k8s api, add it to the cached map and handle the request
				s.CreatePathHandler(*found, path)
				handler.ServeHTTP(w, r)
				return
			}
		}),
	}

	tlsCert := "/tmp/k8s-webhook-server/serving-certs/tls.crt"
	tlsKey := "/tmp/k8s-webhook-server/serving-certs/tls.key"
	if s.dev {
		tlsCert = "/tmp/k8s-webhook-server/serving-certs/server.crt"
		tlsKey = "/tmp/k8s-webhook-server/serving-certs/server.key"
	}

	// Start the server asynchronously
	go func() {
		s.log.Info("Serving resources interceptor webhook server on port 9444")
		if err := s.server.ListenAndServeTLS(tlsCert, tlsKey); err != http.ErrServerClosed {
			s.log.Error(err, "failed to start the resources interceptor webhook server on port 9444")
		}
		close(s.stopped)
	}()

	// Set up signal handling for graceful shutdown
	go s.setupSignalHandler()
}

func (s *WebhookInterceptsAll) setupSignalHandler() {
	// Create a channel to receive OS signals
	sigs := make(chan os.Signal, 1)
	// Register for interrupt and SIGTERM signals
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	// Block until a signal is received
	<-sigs

	ctx := context.Background()
	validationWebhook := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	webhookNamespacedName := &types.NamespacedName{
		Name: os.Getenv("DYNAMIC_WEBHOOK_NAME"),
	}
	err := s.k8sClient.Get(ctx, *webhookNamespacedName, validationWebhook)
	if err != nil {
		s.log.Error(err, "failed to gracefully delete the dynamic remote syncer webhook (fail to get the webhook)")
	}
	err = s.k8sClient.Delete(ctx, validationWebhook)
	if err != nil {
		s.log.Error(err, "failed to gracefully delete the dynamic remote syncer webhook")
	}

	s.Stop()
}

// Stop stops the webhook server
func (s *WebhookInterceptsAll) Stop() {
	s.Lock()
	defer s.Unlock()

	if s.server == nil {
		return
	}

	// Empty the cached path map
	s.pathHandlers = nil

	// Shutdown the server gracefully
	if err := s.server.Shutdown(context.Background()); err != nil {
		s.log.Error(err, "failed to properly stop the resources interceptor webhook server")
	}
	s.log.Info("Resources interceptor webhook server successfully stopped")
	<-s.stopped
	s.server = nil
}

// CreatePathHandler creates a new path handler instance for the given namespace and name
func (s *WebhookInterceptsAll) CreatePathHandler(interceptor syngit.RemoteSyncer, path string) *DynamicWebhookHandler {
	s.Lock()
	defer s.Unlock()

	// Create a new path handler with the specified namespace and name
	handler := &DynamicWebhookHandler{
		remoteSyncer: interceptor,
		k8sClient:    s.k8sClient,
	}

	// Register the path handler with the server
	s.pathHandlers[path] = handler

	return handler
}

// ServeHTTP implements the http.Handler interface for PathHandler
func (dwc *DynamicWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		log:          dwc.log,
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

// DestroyPathHandler removes the path handler associated with the given namespace and name
func (s *WebhookInterceptsAll) DestroyPathHandler(n types.NamespacedName) {
	s.Lock()
	defer s.Unlock()

	path := "/webhook/" + n.Namespace + "/" + n.Name

	// Unregister the path handler from the server
	delete(s.pathHandlers, path)
}
