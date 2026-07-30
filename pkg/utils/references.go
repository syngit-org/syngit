package utils

import (
	"context"
	"slices"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	authenticationv1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectRef is a kind-agnostic description of a reference site held by a syngit
// object. Its Namespace is already resolved: it is either the one explicitly set
// on the reference, or the namespace of the referencing object.
type ObjectRef struct {
	Namespace string
	Name      string
	Group     string
	Version   string
	Resource  string
	FieldPath *field.Path
}

// Returns the effective namespace of a reference.
//
// An empty refNamespace falls back to ownerNamespace. An empty ownerNamespace
// means that the referencing object is cluster-scoped: it has no namespace to
// fall back to, so the reference must carry one.
func ResolveNamespace(refNamespace, ownerNamespace string, fieldPath *field.Path) (string, error) {
	if refNamespace != "" {
		return refNamespace, nil
	}
	if ownerNamespace == "" {
		return "", syngiterrors.NewMissingRefNamespace(fieldPath.String())
	}
	return ownerNamespace, nil
}

// CheckAccess runs a SubjectAccessReview for the given user against attrs.
func CheckAccess(
	ctx context.Context,
	c client.Client,
	user authenticationv1.UserInfo,
	attrs authv1.ResourceAttributes,
) (bool, error) {
	sar := &authv1.SubjectAccessReview{
		Spec: authv1.SubjectAccessReviewSpec{
			User:               user.Username,
			Groups:             user.Groups,
			UID:                user.UID,
			ResourceAttributes: &attrs,
		},
	}
	if err := c.Create(ctx, sar); err != nil {
		return false, err
	}
	return sar.Status.Allowed, nil
}

// Enumerates every namespace-bearing reference of a RemoteSyncer
// spec, with each namespace already resolved against ownerNamespace. Pass an
// empty ownerNamespace for a cluster-scoped referencing object.
func RemoteSyncerRefs(spec syngit.RemoteSyncerSpec, ownerNamespace string) ([]ObjectRef, error) {
	specPath := field.NewPath("spec")
	refs := []ObjectRef{}

	appendRef := func(refNamespace, name string, gvr [3]string, path *field.Path) error {
		if name == "" {
			return nil
		}
		namespace, err := ResolveNamespace(refNamespace, ownerNamespace, path)
		if err != nil {
			return err
		}
		refs = append(refs, ObjectRef{
			Namespace: namespace,
			Name:      name,
			Group:     gvr[0],
			Version:   gvr[1],
			Resource:  gvr[2],
			FieldPath: path,
		})
		return nil
	}

	syngitGVR := func(resource string) [3]string {
		return [3]string{syngit.GroupVersion.Group, syngit.GroupVersion.Version, resource}
	}
	coreGVR := func(resource string) [3]string {
		return [3]string{"", "v1", resource}
	}

	if ref := spec.DefaultRemoteUserRef; ref != nil {
		if err := appendRef(
			ref.Namespace, ref.Name, syngitGVR("remoteusers"), specPath.Child("defaultRemoteUserRef"),
		); err != nil {
			return nil, err
		}
	}

	if ref := spec.DefaultRemoteTargetRef; ref != nil {
		if err := appendRef(
			ref.Namespace, ref.Name, syngitGVR("remotetargets"), specPath.Child("defaultRemoteTargetRef"),
		); err != nil {
			return nil, err
		}
	}

	for i, ref := range spec.ExcludedFieldsConfigMapsRef {
		if ref == nil {
			continue
		}
		if err := appendRef(
			ref.Namespace, ref.Name, coreGVR("configmaps"), specPath.Child("excludedFieldsConfigMapsRef").Index(i),
		); err != nil {
			return nil, err
		}
	}

	if err := appendRef(
		spec.CABundleSecretRef.Namespace,
		spec.CABundleSecretRef.Name,
		coreGVR("secrets"),
		specPath.Child("caBundleSecretRef"),
	); err != nil {
		return nil, err
	}

	return refs, nil
}

// Checks that user is allowed to get every reference that resolves outside of
// ownerNamespace. References resolving into ownerNamespace are not checked at
// all: holding the referencing object already implies access to its own
// namespace, so the common case costs nothing.
//
// Namespaces listed in exempt are skipped as well. This is how the interception
// runtime lets every user reach the operator-owned objects of the manager
// namespace without granting them access to it.
//
// It returns the first reference the user is not allowed to get, or nil when all
// of them are allowed.
func AuthorizeCrossNamespaceRefs(
	ctx context.Context,
	c client.Client,
	user authenticationv1.UserInfo,
	refs []ObjectRef,
	ownerNamespace string,
	exempt ...string,
) (*ObjectRef, error) {
	for _, ref := range refs {
		if ref.Namespace == ownerNamespace || slices.Contains(exempt, ref.Namespace) {
			continue
		}

		allowed, err := CheckAccess(ctx, c, user, authv1.ResourceAttributes{
			Namespace: ref.Namespace,
			Verb:      "get",
			Group:     ref.Group,
			Version:   ref.Version,
			Resource:  ref.Resource,
			Name:      ref.Name,
		})
		if err != nil {
			return nil, err
		}
		if !allowed {
			return &ref, nil
		}
	}

	return nil, nil
}
