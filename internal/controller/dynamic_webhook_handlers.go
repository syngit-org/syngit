package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	kgiov1 "dams.kgio/kgio/api/v1"
	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type WebhookInterceptsAll struct {
	server       *http.Server
	stopped      chan struct{}
	pathHandlers (map[string]*DynamicWebhookHandler)
	sync.RWMutex
	log *logr.Logger
}

// PathHandler represents an instance of a path handler with a specific namespace and name
type DynamicWebhookHandler struct {
	resourcesInterceptor kgiov1.ResourcesInterceptor
}

// Start starts the webhook server
func (s *WebhookInterceptsAll) Start() {
	var log = logf.Log.WithName("resourcesinterceptor-webhook")
	s.log = &log

	s.Lock()
	defer s.Unlock()

	if s.server != nil {
		return
	}

	s.pathHandlers = make(map[string]*DynamicWebhookHandler)

	// Create the HTTP server
	s.server = &http.Server{
		Addr: ":8443",
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
			}

			// If no handler is found, respond with a 404 Not Found status
			http.NotFound(w, r)
		}),
	}

	tlsCert := "/tmp/k8s-webhook-server/serving-certs/server.crt"
	tlsKey := "/tmp/k8s-webhook-server/serving-certs/server.key"

	// Start the server asynchronously
	go func() {
		s.log.Info("Serving resources interceptor webhook server on port 8443")
		if err := s.server.ListenAndServeTLS(tlsCert, tlsKey); err != http.ErrServerClosed {
			s.log.Error(err, "failed to start the resources interceptor webhook server")
		}
		close(s.stopped)
	}()
}

// Stop stops the webhook server
func (s *WebhookInterceptsAll) Stop() {
	s.Lock()
	defer s.Unlock()

	if s.server == nil {
		return
	}

	// Shutdown the server gracefully
	if err := s.server.Shutdown(context.Background()); err != nil {
		s.log.Error(err, "failed to properly stop the resources interceptor webhook server")
	}
	<-s.stopped
	s.server = nil
}

// CreatePathHandler creates a new path handler instance for the given namespace and name
func (s *WebhookInterceptsAll) CreatePathHandler(interceptor kgiov1.ResourcesInterceptor, path string) *DynamicWebhookHandler {
	s.Lock()
	defer s.Unlock()

	// Create a new path handler with the specified namespace and name
	handler := &DynamicWebhookHandler{
		resourcesInterceptor: interceptor,
	}

	// Register the path handler with the server
	s.pathHandlers[path] = handler

	return handler
}

// ServeHTTP implements the http.Handler interface for PathHandler
func (dwc *DynamicWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var log = logf.Log.WithName("resourcesinterceptor-webhook")

	decoder := json.NewDecoder(r.Body)
	var admissionReviewReq admissionv1.AdmissionReview
	err := decoder.Decode(&admissionReviewReq)
	if err != nil {
		http.Error(w, "Failed to decode admission review request", http.StatusBadRequest)
		return
	}

	// Context variables
	var isAllowed = true
	var isGitPushed = false

	if dwc.resourcesInterceptor.Spec.CommitProcess == kgiov1.CommitOnly {
		isAllowed = false
	}

	var gitRepoPath = "/"
	var gitCommitHash = ""

	// Admission response variables
	var admStatus = "Failure"
	var defaultBlockedMessage = "Internal webhook server error. The resource has not been pushed on the remote git repository."

	if isGitPushed {
		admStatus = "Success"
		if dwc.resourcesInterceptor.Spec.DefaultBlockAppliedMessage != "" {
			defaultBlockedMessage = dwc.resourcesInterceptor.Spec.DefaultBlockAppliedMessage
		} else {
			defaultBlockedMessage = "The resource has correctly been pushed on the remote git repository."
		}
	}

	admissionReviewResp := admissionv1.AdmissionReview{
		Response: &admissionv1.AdmissionResponse{
			UID:     admissionReviewReq.DeepCopy().Request.UID,
			Allowed: isAllowed,
			Result: &v1.Status{
				Status:  admStatus,
				Message: defaultBlockedMessage,
			},
			AuditAnnotations: map[string]string{
				"kgio-git-repo-fqdn":   dwc.resourcesInterceptor.Spec.RemoteRepository,
				"kgio-git-repo-path":   gitRepoPath,
				"kgio-git-commit-hash": gitCommitHash,
			},
		},
	}
	admissionReviewResp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "admission.k8s.io",
		Version: "v1",
		Kind:    "AdmissionReview",
	})
	resp, err := json.Marshal(admissionReviewResp)
	if err != nil {
		log.Error(err, "Failed to marshal admission review response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(resp)
	if err != nil {
		log.Error(err, "Failed to write admission review response")
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
