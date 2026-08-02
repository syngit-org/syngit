package refs

import (
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
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

	if ref := spec.SOPS.SecretRef; ref.Name != "" {
		c.add(ref.Namespace, ref.Name, secretsGVR, c.specPath.Child("sops", "secretRef"))
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
