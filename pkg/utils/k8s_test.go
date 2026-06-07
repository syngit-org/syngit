package utils

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestK8sClientFromContext(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()
	ctx := context.WithValue(context.Background(), K8sClientCtxKey{}, client.Client(fakeClient))

	got := K8sClientFromContext(ctx)
	if got != fakeClient {
		t.Errorf("K8sClientFromContext returned a different client than injected")
	}
}
