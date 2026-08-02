package webhooks

import (
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExtractObjectMetadata(t *testing.T) {
	admReq := &admissionv1.AdmissionRequest{
		Name: "my-object",
		RequestResource: &metav1.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
	}

	md := ExtractObjectMetadata(admReq)

	if md.Name != "my-object" {
		t.Errorf("Name=%q, want my-object", md.Name)
	}
	if md.GVR.Group != "apps" || md.GVR.Version != "v1" || md.GVR.Resource != "deployments" {
		t.Errorf("GVR mismatch: %+v", md.GVR)
	}

	// Mutating the original should not affect the returned copy.
	admReq.RequestResource.Group = "mutated"
	if md.GVR.Group != "apps" {
		t.Errorf("GVR.Group should be a deep copy; mutation leaked: %q", md.GVR.Group)
	}
}
