package utils

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CustomClient struct {
	ctx    context.Context
	client client.Client
}

func (customClient CustomClient) CreateOrUpdate(obj client.Object) error {
	// Create a deep copy of the object to avoid modifying the input
	existingObj := obj.DeepCopyObject().(client.Object)

	// Check if the object exists
	err := customClient.client.Get(customClient.ctx, client.ObjectKeyFromObject(obj), existingObj)
	if errors.IsNotFound(err) {
		// Object doesn't exist, create it
		return customClient.client.Create(customClient.ctx, obj)
	} else if err != nil {
		// Return other errors from GET
		return err
	}

	// Retain the resource version to update the object correctly
	obj.SetResourceVersion(existingObj.GetResourceVersion())

	return customClient.client.Update(customClient.ctx, obj)
}

func (customClient CustomClient) List(namespace string, objList client.ObjectList) error {
	return customClient.client.List(customClient.ctx, objList, &client.ListOptions{Namespace: namespace})
}

func (customClient CustomClient) Get(namespacedName types.NamespacedName, obj client.Object) error {
	return customClient.client.Get(customClient.ctx, namespacedName, obj)
}

func (customClient CustomClient) Delete(obj client.Object) error {
	return customClient.client.Delete(customClient.ctx, obj)
}
