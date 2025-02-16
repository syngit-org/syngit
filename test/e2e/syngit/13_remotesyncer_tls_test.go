/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e_syngit

import (
	"context"
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	. "github.com/syngit-org/syngit/test/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("13 RemoteSyncer TLS insecure & custom CA bundle test", func() {
	ctx := context.TODO()

	const (
		cmName1             = "test-cm13.1"
		cmName2             = "test-cm13.2"
		cmName3             = "test-cm13.3"
		cmName4             = "test-cm13.4"
		cmName5             = "test-cm13.5"
		remoteUserLuffyName = "remoteuser-luffy"
		remoteSyncerName1   = "remotesyncer-test13.1"
		remoteSyncerName2   = "remotesyncer-test13.2"
		remoteSyncerName3   = "remotesyncer-test13.3"
		remoteSyncerName4   = "remotesyncer-test13.4"
		remoteSyncerName5   = "remotesyncer-test13.5"
		secretCa1           = "custom-cabundle1"
		secretCa2           = "custom-cabundle2"
		branch              = "main"
	)

	It("should interact with gitea using the user CA bundle", func() {
		caBundleFile, err := os.ReadFile(os.Getenv("GITEA_TEMP_CERT_DIR") + "/ca.crt")
		By("creating the ca bundle secret in the same namespace")
		secretCreds := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretCa1,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"tls.crt": caBundleFile,
				"tls.key": {},
			},
			Type: "kubernetes.io/tls",
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().Secrets(namespace).Create(ctx,
				secretCreds,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUser & RemoteUserBinding for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyName,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RubAnnotationKeyManaged: "true",
				},
			},
			Spec: syngit.RemoteUserSpec{
				Email:             "sample@email.com",
				GitBaseDomainFQDN: gitP1Fqdn,
				SecretRef: corev1.SecretReference{
					Name: luffySecretName,
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo2)
		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName2,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: branch,
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				CABundleSecretRef: corev1.SecretReference{
					Name: secretCa1,
				},
				DefaultBranch:               branch,
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            repoUrl,
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
						},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"configmaps"},
						},
					},
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Sanji).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName2, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking if the configmap is present on the repo")
		Wait3()
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: giteaBaseNs,
			Name:  repo2,
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the configmap is present on the cluster")
		nnCm := types.NamespacedName{
			Name:      cmName2,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})

	It("should interact with gitea using the user CA bundle in the same namespace", func() {
		caBundleFile, err := os.ReadFile(os.Getenv("GITEA_TEMP_CERT_DIR") + "/ca.crt")
		By("creating the ca bundle secret in the same namespace")
		secretCreds := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretCa2,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"tls.crt": caBundleFile,
				"tls.key": {},
			},
			Type: "kubernetes.io/tls",
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().Secrets(namespace).Create(ctx,
				secretCreds,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUser & RemoteUserBinding for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyName,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RubAnnotationKeyManaged: "true",
				},
			},
			Spec: syngit.RemoteUserSpec{
				Email:             "sample@email.com",
				GitBaseDomainFQDN: gitP1Fqdn,
				SecretRef: corev1.SecretReference{
					Name: luffySecretName,
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo2)
		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName3,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: branch,
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				CABundleSecretRef: corev1.SecretReference{
					Name:      secretCa2,
					Namespace: namespace,
				},
				DefaultBranch:               branch,
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            repoUrl,
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
						},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"configmaps"},
						},
					},
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Sanji).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName3, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking if the configmap is present on the repo")
		Wait3()
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: giteaBaseNs,
			Name:  repo2,
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the configmap is present on the cluster")
		nnCm := types.NamespacedName{
			Name:      cmName3,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})

	It("should interact with gitea using the user CA bundle of the operator namespace", func() {
		caBundleFile, err := os.ReadFile(os.Getenv("GITEA_TEMP_CERT_DIR") + "/ca.crt")
		By("creating the ca bundle secret in the same namespace using the host as name")
		caBundleName := strings.Split(gitP1Fqdn, ":")[0] + "-ca-cert"
		secretCreds := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      caBundleName,
				Namespace: operatorNamespace,
			},
			Data: map[string][]byte{
				"tls.crt": caBundleFile,
				"tls.key": {},
			},
			Type: "kubernetes.io/tls",
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Admin).CoreV1().Secrets(operatorNamespace).Create(ctx,
				secretCreds,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUser & RemoteUserBinding for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyName,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RubAnnotationKeyManaged: "true",
				},
			},
			Spec: syngit.RemoteUserSpec{
				Email:             "sample@email.com",
				GitBaseDomainFQDN: gitP1Fqdn,
				SecretRef: corev1.SecretReference{
					Name: luffySecretName,
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo2)
		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName4,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: branch,
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				DefaultBranch:               branch,
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            repoUrl,
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
						},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"configmaps"},
						},
					},
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Sanji).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName4, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking if the configmap is present on the repo")
		Wait3()
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: giteaBaseNs,
			Name:  repo2,
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the configmap is present on the cluster")
		nnCm := types.NamespacedName{
			Name:      cmName4,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("deleting the ca bundle secret in the same namespace using the host as name")
		Eventually(func() bool {
			err = sClient.KAs(Admin).CoreV1().Secrets(operatorNamespace).Delete(ctx,
				caBundleName,
				metav1.DeleteOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())
	})

	It("should get a x509 error ", func() {
		By("creating the RemoteUser & RemoteUserBinding for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyName,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RubAnnotationKeyManaged: "true",
				},
			},
			Spec: syngit.RemoteUserSpec{
				Email:             "sample@email.com",
				GitBaseDomainFQDN: gitP1Fqdn,
				SecretRef: corev1.SecretReference{
					Name: luffySecretName,
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo2)
		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName5,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: branch,
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				DefaultBranch:               branch,
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            repoUrl,
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
						},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"configmaps"},
						},
					},
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Sanji).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName5, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err != nil && strings.Contains(err.Error(), x509ErrorMessage)
		}, timeout, interval).Should(BeTrue())

	})
})
