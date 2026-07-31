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

// The GVRs that syngit objects can reference. A reference site names one of
// them; nothing else in this package needs to know about group versions.
var (
	remoteUsersGVR   = [3]string{syngit.GroupVersion.Group, syngit.GroupVersion.Version, "remoteusers"}
	remoteTargetsGVR = [3]string{syngit.GroupVersion.Group, syngit.GroupVersion.Version, "remotetargets"}
	configMapsGVR    = [3]string{"", "v1", "configmaps"}
	secretsGVR       = [3]string{"", "v1", "secrets"}
)

// refCollector accumulates the references of a single object, resolving each
// namespace against the same owner namespace. It exists so that every
// enumerator below is a flat list of "this field points at that kind".
type refCollector struct {
	ownerNamespace string
	specPath       *field.Path
	refs           []ObjectRef
	err            error
}

func newRefCollector(ownerNamespace string) *refCollector {
	return &refCollector{
		ownerNamespace: ownerNamespace,
		specPath:       field.NewPath("spec"),
		refs:           []ObjectRef{},
	}
}

// add records one reference. An unset reference (empty name) contributes
// nothing: every reference site handled here is optional at the API level.
// The first error is kept and short-circuits every later call, so that
// enumerators can stay branch-free and check err once at the end.
func (c *refCollector) add(refNamespace, name string, gvr [3]string, path *field.Path) {
	if c.err != nil || name == "" {
		return
	}
	namespace, err := ResolveNamespace(refNamespace, c.ownerNamespace, path)
	if err != nil {
		c.err = err
		return
	}
	c.refs = append(c.refs, ObjectRef{
		Namespace: namespace,
		Name:      name,
		Group:     gvr[0],
		Version:   gvr[1],
		Resource:  gvr[2],
		FieldPath: path,
	})
}

func (c *refCollector) result() ([]ObjectRef, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.refs, nil
}

// Enumerates every namespace-bearing reference of a RemoteSyncer
// spec, with each namespace already resolved against ownerNamespace. Pass an
// empty ownerNamespace for a cluster-scoped referencing object.
func RemoteSyncerRefs(spec syngit.RemoteSyncerSpec, ownerNamespace string) ([]ObjectRef, error) {
	c := newRefCollector(ownerNamespace)

	if ref := spec.DefaultRemoteUserRef; ref != nil {
		c.add(ref.Namespace, ref.Name, remoteUsersGVR, c.specPath.Child("defaultRemoteUserRef"))
	}

	if ref := spec.DefaultRemoteTargetRef; ref != nil {
		c.add(ref.Namespace, ref.Name, remoteTargetsGVR, c.specPath.Child("defaultRemoteTargetRef"))
	}

	for i, ref := range spec.ExcludedFieldsConfigMapsRef {
		if ref == nil {
			continue
		}
		c.add(ref.Namespace, ref.Name, configMapsGVR, c.specPath.Child("excludedFieldsConfigMapsRef").Index(i))
	}

	// CABundleSecretRef is a value, not a pointer: it is never nil, and an
	// unset one is the zero struct. Testing its name keeps this site visibly
	// guarded like the pointer refs above.
	if ref := spec.CABundleSecretRef; ref.Name != "" {
		c.add(ref.Namespace, ref.Name, secretsGVR, c.specPath.Child("caBundleSecretRef"))
	}

	return c.result()
}

// Enumerates every namespace-bearing reference of a RemoteUser spec, with each
// namespace already resolved against ownerNamespace.
func RemoteUserRefs(spec syngit.RemoteUserSpec, ownerNamespace string) ([]ObjectRef, error) {
	c := newRefCollector(ownerNamespace)

	if ref := spec.SecretRef; ref.Name != "" {
		c.add(ref.Namespace, ref.Name, secretsGVR, c.specPath.Child("secretRef"))
	}

	return c.result()
}

// Enumerates every namespace-bearing reference of a RemoteUserBinding spec,
// with each namespace already resolved against ownerNamespace.
func RemoteUserBindingRefs(spec syngit.RemoteUserBindingSpec, ownerNamespace string) ([]ObjectRef, error) {
	c := newRefCollector(ownerNamespace)

	for i, ref := range spec.RemoteUserRefs {
		c.add(ref.Namespace, ref.Name, remoteUsersGVR, c.specPath.Child("remoteUserRefs").Index(i))
	}

	for i, ref := range spec.RemoteTargetRefs {
		c.add(ref.Namespace, ref.Name, remoteTargetsGVR, c.specPath.Child("remoteTargetRefs").Index(i))
	}

	return c.result()
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
	crossNamespace := make([]ObjectRef, 0, len(refs))
	for _, ref := range refs {
		if ref.Namespace == ownerNamespace || slices.Contains(exempt, ref.Namespace) {
			continue
		}
		crossNamespace = append(crossNamespace, ref)
	}

	return AuthorizeRefs(ctx, c, user, crossNamespace)
}

// Checks that user is allowed to get every one of the given references,
// wherever they resolve. Callers that must not pay for same-namespace
// references use AuthorizeCrossNamespaceRefs instead; callers that gate access
// to the referenced object itself (a RemoteUser's credentials, the RemoteUsers
// a RemoteUserBinding grants) use this one, so that authority over the
// referencing object never implies authority over what it points at.
//
// It returns the first reference the user is not allowed to get, or nil when all
// of them are allowed.
func AuthorizeRefs(
	ctx context.Context,
	c client.Client,
	user authenticationv1.UserInfo,
	refs []ObjectRef,
) (*ObjectRef, error) {
	for _, ref := range refs {
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
