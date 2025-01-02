package utils

import (
	"fmt"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

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

func GetObjectFromWebhookRequest(decoder *admission.Decoder, obj runtime.Object, req admission.Request) error {

	if string(req.Operation) != "DELETE" {
		err := decoder.Decode(req, obj)
		if err != nil {
			return err
		}
	} else {
		err := decoder.DecodeRaw(req.OldObject, obj)
		if err != nil {
			return err
		}
	}
	return nil
}
