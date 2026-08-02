package interceptor

import (
	"context"

	"github.com/syngit-org/syngit/pkg/kube"
	"github.com/syngit-org/syngit/pkg/refs"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// resolveRefs fetches every object a list of references points at, resolving
// each namespace against ownerNamespace. References that do not resolve to an
// existing object are skipped: a dangling reference simply contributes nothing.
func resolveRefs[T any, PT interface {
	*T
	client.Object
}](
	ctx context.Context,
	objectRefs []corev1.ObjectReference,
	ownerNamespace string,
	fieldName string,
	newObject func() PT,
) ([]PT, error) {
	k8sClient := kube.ClientFromContext(ctx)
	resolved := make([]PT, 0, len(objectRefs))

	for i, ref := range objectRefs {
		if ref.Name == "" {
			continue
		}
		namespace, err := refs.ResolveNamespace(
			ref.Namespace, ownerNamespace, field.NewPath("spec", fieldName).Index(i),
		)
		if err != nil {
			return nil, err
		}

		object := newObject()
		if err := k8sClient.Get(ctx,
			types.NamespacedName{Namespace: namespace, Name: ref.Name}, object,
		); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		resolved = append(resolved, object)
	}

	return resolved, nil
}
