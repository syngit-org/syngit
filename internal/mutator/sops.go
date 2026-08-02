package mutator

import (
	"errors"
	"fmt"

	sopsprovider "github.com/syngit-org/syngit-provider-sops/pkg"
	"github.com/syngit-org/syngit/internal/walker"
	features "github.com/syngit-org/syngit/pkg/feature"
	"github.com/syngit-org/syngit/pkg/refs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// sopsConfigFile is the SOPS configuration of the remote repository. It holds
// the creation rules: which paths get encrypted, with which recipients, and how
// much of each document is covered.
const sopsConfigFile = ".sops.yaml"

// sopsTransform builds the content transform that encrypts every document
// pushed by this syncer, or returns nil when the SOPS provider is off.
//
// Everything that can fail regardless of the document being written is resolved
// here, once: the .sops.yaml of the repository and the age identities. The
// returned closure is then called per (document, path) by the placement phase.
func sopsTransform(rc RenderContext) (walker.DocTransform, error) {
	sops := rc.Params.Syncer.Spec.SOPS
	if !features.LoadedFeatureGates.Enabled(features.SopsEncryption) || !sops.Enabled {
		return nil, nil
	}

	// A syncer that asks for SOPS against a repository that carries no
	// .sops.yaml has nothing to encrypt with. That is broken configuration
	// rather than a scoping decision, so fail instead of silently pushing the
	// manifests in cleartext.
	sopsYAML, err := walker.ReadWorktreeFile(rc.Worktree, sopsConfigFile)
	if err != nil {
		return nil, fmt.Errorf(
			"spec.sops.enabled is set but the repository has no %s to read the creation rules from: %w",
			sopsConfigFile, err,
		)
	}

	identities, err := sopsIdentities(rc)
	if err != nil {
		return nil, err
	}

	return func(relPath string, existing, content []byte) ([]byte, error) {
		rules, err := sopsprovider.LoadCreationRule(sopsYAML, relPath)
		if err != nil {
			// The path falls outside every creation rule: the user scoped this
			// document out of their encryption, so push it as it is.
			if errors.Is(err, sopsprovider.ErrNoCreationRule) {
				return content, nil
			}
			return nil, fmt.Errorf("failed to load the %s creation rule for %s: %w", sopsConfigFile, relPath, err)
		}

		cfg := sopsprovider.Config{Rules: rules, Identities: identities}

		// With the manifest already in the repository and a key to open it, only
		// the values that really moved get new ciphertext, so an unchanged object
		// produces no diff and no commit. The provider falls back to a full
		// encryption on its own when it cannot open the existing manifest.
		var doc *sopsprovider.EncryptedDocument
		if len(existing) > 0 && identities != nil {
			doc, err = sopsprovider.EncryptWithExisting(content, existing, cfg)
		} else {
			doc, err = sopsprovider.EncryptYAML(content, cfg)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt %s with SOPS: %w", relPath, err)
		}

		// syngit locates a document in the repository by its Kubernetes identity.
		// An encrypted metadata.name would make every later push fail to find this
		// document and append a duplicate next to it instead of replacing it.
		if !doc.KubernetesIdentityVisible {
			return nil, fmt.Errorf(
				"the %s creation rule matching %s encrypts the identity of the object: "+
					"its encrypted_regex must leave apiVersion, kind and metadata.name in cleartext "+
					"so that syngit can find the document again",
				sopsConfigFile, relPath,
			)
		}

		return []byte(doc.RawYAML), nil
	}, nil
}

// sopsIdentities reads the age private key the syncer points at. It returns nil
// when no secret is referenced: encrypting only needs the .sops.yaml recipients,
// and the identities are what buys the stable diff on top.
func sopsIdentities(rc RenderContext) (sopsprovider.IdentitySource, error) {
	ref := rc.Params.Syncer.Spec.SOPS.SecretRef
	if ref.Name == "" {
		return nil, nil
	}

	fieldPath := field.NewPath("spec", "sops", "secretRef")
	namespace, err := refs.ResolveNamespace(ref.Namespace, rc.Params.Syncer.RefOwnerNamespace, fieldPath)
	if err != nil {
		return nil, err
	}

	// Downgrading to a keyless encryption here would silently rewrite every
	// encrypted value on every push, so a referenced secret that cannot be read
	// is an error.
	if rc.Cluster == nil {
		return nil, fmt.Errorf("%s is set but no cluster reader is available to get the secret", fieldPath)
	}

	secret := &corev1.Secret{}
	if err := rc.Cluster.Get(rc.Ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, secret); err != nil {
		return nil, fmt.Errorf("failed to get the SOPS secret %s/%s: %w", namespace, ref.Name, err)
	}

	identities, err := sopsprovider.AgeIdentitiesFromSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to read the age identities from the secret %s/%s: %w", namespace, ref.Name, err)
	}

	return identities, nil
}
