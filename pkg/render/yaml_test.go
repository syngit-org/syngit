package render

import (
	"testing"
)

func TestJSONToMap(t *testing.T) {
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
			got, err := JSONToMap(tc.input)
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
