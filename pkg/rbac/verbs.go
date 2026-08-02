package rbac

import (
	"fmt"

	admissionv1 "k8s.io/api/admissionregistration/v1"
)

// OperationToVerb maps an admission operation to the RBAC verbs that cover it.
// An update is two verbs, because a client may reach it through either.
func OperationToVerb(operation admissionv1.OperationType) ([]string, error) {
	switch operation {
	case admissionv1.Create:
		return []string{"create"}, nil
	case admissionv1.Delete:
		return []string{"delete"}, nil
	case admissionv1.Update:
		return []string{"update", "patch"}, nil
	case admissionv1.Connect:
		return []string{"connect"}, nil
	default:
		return nil, fmt.Errorf("unsupported operation: %v", operation)
	}
}
