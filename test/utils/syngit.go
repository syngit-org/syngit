package utils

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func getObjectMetadata(obj runtime.Object) (metav1.Object, error) {
	// Use meta.Accessor to get the metadata
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to access metadata: %w", err)
	}
	return metadata, nil
}

func AreObjectsUploaded(repo Repo, objects []runtime.Object) bool {
	for _, object := range objects {
		isObjInRepo, err := IsObjectInRepo(repo, object)
		if err != nil || !isObjInRepo {
			return false
		}
	}
	return true
}
