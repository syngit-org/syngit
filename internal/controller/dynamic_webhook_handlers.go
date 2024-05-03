package controller

import (
	"context"
	"net/http"
	"sync"

	kgiov1 "dams.kgio/kgio/api/v1"
	"k8s.io/apimachinery/pkg/types"
)

type WebhookInterceptsAll struct {
	server       *http.Server
	stopped      chan struct{}
	pathHandlers (map[string]*DynamicWebhookHandler)
	sync.Mutex
}

// PathHandler represents an instance of a path handler with a specific namespace and name
type DynamicWebhookHandler struct {
	resourcesInterceptor kgiov1.ResourcesInterceptor
}

// Start starts the webhook server
func (s *WebhookInterceptsAll) Start() {
	s.Lock()
	defer s.Unlock()

	if s.server != nil {
		return
	}

	s.pathHandlers = make(map[string]*DynamicWebhookHandler)

	// Create the HTTP server
	s.server = &http.Server{
		Addr: ":8080", // Define your desired port
		// Define your HTTP handler here
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get the path from the request URL
			path := r.URL.Path

			// Find the appropriate path handler based on the request path
			handler, ok := s.pathHandlers[path]

			// If a handler is found, invoke it
			if ok {
				handler.ServeHTTP(w, r)
				return
			}

			// If no handler is found, respond with a 404 Not Found status
			http.NotFound(w, r)
		}),
	}

	// Start the server asynchronously
	go func() {
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			// Handle error
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
		// Handle error
	}
	<-s.stopped
	s.server = nil
}

// CreatePathHandler creates a new path handler instance for the given namespace and name
func (s *WebhookInterceptsAll) CreatePathHandler(interceptor kgiov1.ResourcesInterceptor) *DynamicWebhookHandler {
	path := "/webhook/" + interceptor.Namespace + "/" + interceptor.Name

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
	// Access the namespace and name from the PathHandler
	namespace := dwc.resourcesInterceptor.Namespace
	name := dwc.resourcesInterceptor.Name

	// Check conditions to determine whether to deny the request
	if true {
		// Respond with HTTP 403 Forbidden status code
		http.Error(w, "Access Denied Message", http.StatusForbidden)
		return
	}

	// Continue with normal handling if the request is not denied
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Webhook received for namespace: " + namespace + ", name: " + name))
}

// DestroyPathHandler removes the path handler associated with the given namespace and name
func (s *WebhookInterceptsAll) DestroyPathHandler(n types.NamespacedName) {
	path := "/webhook/" + n.Namespace + "/" + n.Name

	// Unregister the path handler from the server
	delete(s.pathHandlers, path)
}
