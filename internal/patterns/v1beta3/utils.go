package v1beta3

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func generateName(ctx context.Context, client client.Client, object client.Object, suffixNumber int) (string, error) {
	newName := object.GetName()
	if suffixNumber > 0 {
		newName = fmt.Sprintf("%s-%d", object.GetName(), suffixNumber)
	}
	webhookNamespacedName := &types.NamespacedName{
		Name:      newName,
		Namespace: object.GetNamespace(),
	}
	getErr := client.Get(ctx, *webhookNamespacedName, object)
	if getErr == nil {
		return generateName(ctx, client, object, suffixNumber+1)
	} else {
		if strings.Contains(getErr.Error(), "not found") {
			return newName, nil
		}
		return "", getErr
	}
}
