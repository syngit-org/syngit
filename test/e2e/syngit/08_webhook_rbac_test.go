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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta2"
	. "github.com/syngit-org/syngit/test/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("08 Webhook rbac checker", func() {

	const (
		remoteUserLuffyName = "remoteuser-luffy"
		remoteUserBrookName = "remoteuser-brook"
		remoteSyncer1Name   = "remotesyncer-test8.1"
		remoteSyncer2Name   = "remotesyncer-test8.2"
		cmName              = "test-cm8"
		secretName          = "test-secret8"
	)
	ctx := context.TODO()

	It("should deny the resource because of lack of permissions", func() {

		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		By("creating the RemoteUser & RemoteUserBinding for Brook (test the RUB creation without the right permissions)")
		brookSecretName := string(Brook) + "-creds"
		remoteUserBrook := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBrookName,
				Namespace: namespace,
				Annotations: map[string]string{
					"syngit.io/associated-remote-userbinding": "true",
				},
			},
			Spec: syngit.RemoteUserSpec{
				Email:             "sample@email.com",
				GitBaseDomainFQDN: gitP1Fqdn,
				SecretRef: corev1.SecretReference{
					Name: brookSecretName,
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Brook).CreateOrUpdate(remoteUserBrook)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		repoUrl := "http://" + gitP1Fqdn + "/syngituser/blue.git"
		By("creating the RemoteSyncer for ConfigMaps")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncer1Name,
				Namespace: namespace,
			},
			Spec: syngit.RemoteSyncerSpec{
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				ProcessMode:                 syngit.CommitApply,
				PushMode:                    syngit.SameBranch,
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
		err = sClient.As(Brook).CreateOrUpdate(remotesyncer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(rsPermissionsDeniedMessage))

		By("creating a test configmap")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is not present in the repo")
		Wait3()
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: "syngituser",
			Name:  "blue",
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())

		By("checking that the configmap is present on the cluster")
		Wait3()
		nnCm := types.NamespacedName{
			Name:      cmName,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})

	It("should create the resource using the minimum permissions", func() {

		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		By("creating the RemoteUser & RemoteUserBinding for Brook (test the RUB creation without the right permissions)")
		brookSecretName := string(Brook) + "-creds"
		remoteUserBrook := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBrookName,
				Namespace: namespace,
				Annotations: map[string]string{
					"syngit.io/associated-remote-userbinding": "true",
				},
			},
			Spec: syngit.RemoteUserSpec{
				Email:             "sample@email.com",
				GitBaseDomainFQDN: gitP1Fqdn,
				SecretRef: corev1.SecretReference{
					Name: brookSecretName,
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Brook).CreateOrUpdate(remoteUserBrook)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		repoUrl := "http://" + gitP1Fqdn + "/syngituser/blue.git"
		By("creating a wrong RemoteSyncer for Secrets")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncer2Name,
				Namespace: namespace,
			},
			Spec: syngit.RemoteSyncerSpec{
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				ProcessMode:                 syngit.CommitApply,
				PushMode:                    syngit.SameBranch,
				RemoteRepository:            repoUrl,
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
							admissionv1.Delete,
						},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"secrets"},
						},
					},
					},
				},
			},
		}
		err = sClient.As(Brook).CreateOrUpdate(remotesyncer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(rsPermissionsDeniedMessage))
		Expect(err.Error()).To(ContainSubstring("DELETE"))

		By("creating a good RemoteSyncer for Secrets")
		remotesyncer = &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncer2Name,
				Namespace: namespace,
			},
			Spec: syngit.RemoteSyncerSpec{
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				ProcessMode:                 syngit.CommitApply,
				PushMode:                    syngit.SameBranch,
				RemoteRepository:            repoUrl,
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
						},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"secrets"},
						},
					},
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Brook).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test secret")
		Wait3()
		secret := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			StringData: map[string]string{"test": "test1"},
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Brook).CoreV1().Secrets(namespace).Create(ctx,
				secret,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the secret present in the repo")
		Wait3()
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: "syngituser",
			Name:  "blue",
		}
		exists, err := IsObjectInRepo(*repo, secret)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the secret is present on the cluster")
		nnSecret := types.NamespacedName{
			Name:      secretName,
			Namespace: namespace,
		}
		getSecret := &corev1.Secret{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnSecret, getSecret)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})

})
