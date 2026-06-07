package utils

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type K8sClientCtxKey struct{}

func K8sClientFromContext(ctx context.Context) client.Client {
	return ctx.Value(K8sClientCtxKey{}).(client.Client)
}
