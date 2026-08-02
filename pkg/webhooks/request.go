package webhooks

import (
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// DecodeObject decodes the object carried by an admission request into obj. A
// delete request carries it in OldObject, since the new object is already gone.
func DecodeObject(decoder admission.Decoder, obj runtime.Object, req admission.Request) error {
	if req.Operation != admissionv1.Delete {
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

// ObjectMetadata identifies the object an admission request is about.
type ObjectMetadata struct {
	GVR schema.GroupVersionResource
	// Name of the intercepted object.
	Name string
	// Namespace of the intercepted object. Empty when it is cluster-scoped.
	Namespace string
}

// ExtractObjectMetadata reads the identity of the intercepted object off an
// admission request. The GVR is deep-copied, so the result outlives the request.
func ExtractObjectMetadata(admissionRequest *admissionv1.AdmissionRequest) ObjectMetadata {
	interceptedGVR := (*schema.GroupVersionResource)(admissionRequest.RequestResource.DeepCopy())

	return ObjectMetadata{
		Name:      admissionRequest.Name,
		Namespace: admissionRequest.Namespace,
		GVR:       *interceptedGVR,
	}
}
