package utils

import (
	"reflect"
	"testing"

	admissionv1 "k8s.io/api/admissionregistration/v1"
)

func TestOperationToVerb(t *testing.T) {
	tests := []struct {
		name      string
		operation admissionv1.OperationType
		want      []string
		wantErr   bool
	}{
		{"Create maps to create", admissionv1.Create, []string{"create"}, false},
		{"Delete maps to delete", admissionv1.Delete, []string{"delete"}, false},
		{"Update maps to update+patch", admissionv1.Update, []string{"update", "patch"}, false},
		{"Connect maps to connect", admissionv1.Connect, []string{"connect"}, false},
		{"unknown operation errors", admissionv1.OperationType("Bogus"), nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := OperationToVerb(tc.operation)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
