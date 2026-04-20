package interceptor

import (
	"context"
	"testing"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewRemoteSyncerStatusUpdater(t *testing.T) {
	admReq := &admissionv1.AdmissionRequest{
		Name: "my-object",
		Resource: metav1.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
		UserInfo: authenticationv1.UserInfo{Username: "alice"},
	}
	rs := syngit.RemoteSyncer{}
	rs.Name = "my-syncer" // nolint:goconst

	updater := NewRemoteSyncerStatusUpdater(admReq, rs)

	if updater.remoteSyncer.Name != "my-syncer" { // nolint:goconst
		t.Errorf("remoteSyncer.Name=%q, want my-syncer", updater.remoteSyncer.Name)
	}
	if updater.group != "apps" || updater.version != "v1" || updater.resource != "deployments" { // nolint:goconst
		t.Errorf("GVR mismatch: %s/%s/%s", updater.group, updater.version, updater.resource)
	}
	if updater.resourceName != "my-object" {
		t.Errorf("resourceName=%q, want my-object", updater.resourceName)
	}
	if updater.userInfo.Username != "alice" {
		t.Errorf("userInfo.Username=%q, want alice", updater.userInfo.Username)
	}
}

func TestNewRemoteSyncerConditionUpdater(t *testing.T) {
	rs := syngit.RemoteSyncer{}
	rs.Name = "my-syncer" // nolint:goconst

	updater := NewRemoteSyncerConditionUpdater(rs)
	if updater.remoteSyncer.Name != "my-syncer" { // nolint:goconst
		t.Errorf("remoteSyncer.Name=%q, want my-syncer", updater.remoteSyncer.Name)
	}
}

func TestBuildErrorCondition(t *testing.T) {
	c := BuildErrorCondition("boom")

	if c.Type != "Synced" {
		t.Errorf("Type=%q, want Synced", c.Type)
	}
	if c.Status != metav1.ConditionFalse {
		t.Errorf("Status=%q, want False", c.Status)
	}
	if c.Reason != "WebhookHandlerError" {
		t.Errorf("Reason=%q, want WebhookHandlerError", c.Reason)
	}
	if c.Message != "boom" {
		t.Errorf("Message=%q, want boom", c.Message)
	}
	if c.LastTransitionTime.IsZero() {
		t.Errorf("LastTransitionTime should be set")
	}
}

func TestBuildSuccessCondition(t *testing.T) {
	c := BuildSuccessCondition("all good")

	if c.Type != "Synced" {
		t.Errorf("Type=%q, want Synced", c.Type)
	}
	if c.Status != metav1.ConditionTrue {
		t.Errorf("Status=%q, want True", c.Status)
	}
	if c.Reason != "WebhookHandlerSucceeded" {
		t.Errorf("Reason=%q, want WebhookHandlerSucceeded", c.Reason)
	}
	if c.Message != "all good" {
		t.Errorf("Message=%q, want 'all good'", c.Message)
	}
	if c.LastTransitionTime.IsZero() {
		t.Errorf("LastTransitionTime should be set")
	}
}

func TestK8sClientFromContext(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()
	ctx := context.WithValue(context.Background(), k8sClientCtxKey{}, client.Client(fakeClient))

	got := K8sClientFromContext(ctx)
	if got != fakeClient {
		t.Errorf("K8sClientFromContext returned a different client than injected")
	}
}
