package interceptor

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	"github.com/syngit-org/syngit/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func CABundleBuilder(
	ctx context.Context,
	remoteSyncer syngit.RemoteSyncer,
	remoteSyncerRemoteRepoUrl *url.URL,
) ([]byte, error) {
	// Step 1: Search for the global CA Bundle of the server located in the syngit namespace
	caBundle, err := FindGlobalCABundle(ctx, strings.Split(remoteSyncerRemoteRepoUrl.Host, ":")[0])
	if err != nil && errors.Is(err, ErrCaSecretWrongType) {
		return nil, err
	}

	// Step 2: Search for a specific CA Bundle, by default in the current namespace
	caBundleSecretRef := remoteSyncer.Spec.CABundleSecretRef
	if caBundleSecretRef.Name == "" {
		return caBundle, nil
	}
	ns, err := utils.ResolveNamespace(
		caBundleSecretRef.Namespace,
		remoteSyncer.Namespace,
		field.NewPath("spec", "caBundleSecretRef"),
	)
	if err != nil {
		return nil, err
	}
	caBundleRsy, err := FindCABundle(ctx, ns, caBundleSecretRef.Name)
	if err != nil {
		return nil, err
	}
	if caBundleRsy != nil {
		caBundle = caBundleRsy
	}

	return caBundle, nil
}

var ErrCaSecretWrongType = errors.New("the CA bundle secret must be of type \"kubernetes.io/ts\"") // nolint:lll

func FindGlobalCABundle(ctx context.Context, host string) ([]byte, error) {
	return FindCABundle(ctx, os.Getenv("MANAGER_NAMESPACE"), host+"-ca-cert")
}

func FindCABundle(ctx context.Context, namespace string, name string) ([]byte, error) {
	if name == "" {
		return nil, nil
	}

	globalNamespacedName := types.NamespacedName{Namespace: namespace, Name: name}
	caBundleSecret := &corev1.Secret{}

	k8sClient := utils.K8sClientFromContext(ctx)

	err := k8sClient.Get(ctx, globalNamespacedName, caBundleSecret)
	if err != nil {
		return nil, err
	}
	if caBundleSecret.Type != "kubernetes.io/tls" {
		return nil, ErrCaSecretWrongType
	}
	return caBundleSecret.Data["tls.crt"], nil
}
