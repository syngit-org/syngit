package interceptor

import (
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvertObjectJSONToYAMLMap(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
		check   func(t *testing.T, m map[string]interface{})
	}{
		{
			name:  "valid JSON is unmarshalled",
			input: []byte(`{"kind":"Pod","metadata":{"name":"demo"}}`),
			check: func(t *testing.T, m map[string]interface{}) {
				if m["kind"] != "Pod" {
					t.Errorf("kind=%v, want Pod", m["kind"])
				}
				md, _ := m["metadata"].(map[string]interface{})
				if md["name"] != "demo" {
					t.Errorf("metadata.name=%v, want demo", md["name"])
				}
			},
		},
		{
			name:    "empty input errors",
			input:   []byte(""),
			wantErr: true,
		},
		{
			name:    "invalid JSON errors",
			input:   []byte(`{not json`),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertObjectJSONToYAMLMap(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestContainsDeletionTimestamp(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want bool
	}{
		{
			name: "present",
			data: map[string]interface{}{
				"metadata": map[string]interface{}{
					"deletionTimestamp": "2024-01-01T00:00:00Z",
				},
			},
			want: true,
		},
		{
			name: "absent",
			data: map[string]interface{}{
				"metadata": map[string]interface{}{"name": "demo"},
			},
			want: false,
		},
		{
			name: "metadata missing",
			data: map[string]interface{}{"kind": "Pod"},
			want: false,
		},
		{
			name: "metadata of wrong type",
			data: map[string]interface{}{"metadata": "not-a-map"},
			want: false,
		},
		{
			name: "empty map",
			data: map[string]interface{}{},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ContainsDeletionTimestamp(tc.data); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtractObjectMetadataFromAdmissionRequest(t *testing.T) {
	admReq := &admissionv1.AdmissionRequest{
		Name: "my-object",
		RequestResource: &metav1.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
	}

	md := ExtractObjectMetadataFromAdmissionRequest(admReq)

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
