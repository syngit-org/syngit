package kube

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClientFromContext(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()
	ctx := context.WithValue(context.Background(), ClientCtxKey{}, client.Client(fakeClient))

	got := ClientFromContext(ctx)
	if got != fakeClient {
		t.Errorf("ClientFromContext returned a different client than injected")
	}
}
