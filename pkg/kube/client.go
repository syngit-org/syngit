package kube

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClientCtxKey is the context key under which the kubernetes client is carried.
type ClientCtxKey struct{}

// ClientFromContext returns the kubernetes client carried by ctx. It panics
// when there is none: every entrypoint that starts a request injects one, so an
// absent client is a wiring bug, not a runtime condition.
func ClientFromContext(ctx context.Context) client.Client {
	return ctx.Value(ClientCtxKey{}).(client.Client)
}
