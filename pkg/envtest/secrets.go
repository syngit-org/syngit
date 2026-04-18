package envtest

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewBasicAuthSecret builds a kubernetes.io/basic-auth Secret for a GitUser,
// suitable for RemoteUser.Spec.SecretRef.
func NewBasicAuthSecret(user GitUser, name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeBasicAuth,
		Data: map[string][]byte{
			corev1.BasicAuthUsernameKey: []byte(user.Username),
			corev1.BasicAuthPasswordKey: []byte(user.Password),
		},
	}
}

// NewTLSSecret builds a kubernetes.io/tls Secret containing the server's CA
// certificate as tls.crt, suitable for RemoteSyncer.Spec.CABundleSecretRef.
// Only the CA certificate is populated; tls.key is left empty because the
// consumer only needs to verify the server identity.
func (gs *GitServer) NewTLSSecret(name, namespace string) *corev1.Secret {
	ca := gs.CACert()
	if ca == nil {
		return nil
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       ca,
			corev1.TLSPrivateKeyKey: {},
		},
	}
}
