// Package utils hosts the shared infrastructure for the end-to-end suite
// in test/e2e/syngit/tests. It exposes:
//   - the 3-user identity model (Admin, Developer, Restricted)
//   - the Suite type that owns the envtest environment + dual GitServers
//   - the Fixture type used by individual specs for per-spec isolation
package utils

import "time"

// Cluster-level constants shared across specs.
const (
	OperatorNamespace     = "syngit"
	DynamicWebhookName    = "syngit-dynamic-remotesyncer-webhook"
	RepoOwner             = "syngituser"
	RestrictedClusterRole = "e2e-restricted-role"
	DeveloperBindingName  = "e2e-developer-admin"
	RestrictedBindingName = "e2e-restricted-binding"

	// DefaultDeniedMessage is what RemoteSyncer returns when blocking.
	DefaultDeniedMessage = "DENIED ON PURPOSE"
	// X509ErrorMessage is the expected substring when TLS verification fails.
	X509ErrorMessage = "x509: certificate signed by unknown authority"
	// NotFoundMessage is the expected substring for k8s NotFound errors.
	NotFoundMessage = "not found"

	DefaultTimeout  = 60 * time.Second
	DefaultInterval = 250 * time.Millisecond
)
