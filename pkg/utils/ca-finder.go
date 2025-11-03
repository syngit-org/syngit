package utils

import (
	"context"
	"errors"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const CaSecretWrongTypeErrorMessage = "the CA bundle secret must be of type \"kubernetes.io/ts\""

func FindGlobalCABundle(c client.Client, host string) ([]byte, error) {
	return FindCABundle(c, os.Getenv("MANAGER_NAMESPACE"), host+"-ca-cert")
}

func FindCABundle(c client.Client, namespace string, name string) ([]byte, error) {
	if name == "" {
		return nil, nil
	}

	ctx := context.Background()
	globalNamespacedName := types.NamespacedName{Namespace: namespace, Name: name}
	caBundleSecret := &corev1.Secret{}

	err := c.Get(ctx, globalNamespacedName, caBundleSecret)
	if err != nil {
		return nil, err
	}
	if caBundleSecret.Type != "kubernetes.io/tls" {
		return nil, errors.New(CaSecretWrongTypeErrorMessage)
	}
	return caBundleSecret.Data["tls.crt"], nil
}
