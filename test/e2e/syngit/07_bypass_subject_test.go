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
	syngit "github.com/syngit-org/syngit/api/v1beta2"
	. "github.com/syngit-org/syngit/test/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("07 Subject bypasses interception", func() {

	const (
		remoteUserLuffyName = "remoteuser-luffy"
		remoteSyncer1Name   = "remotesyncer-test7.1"
		remoteSyncer2Name   = "remotesyncer-test7.2"
		cmName              = "test-cm7"
	)

	It("should apply the resource on the cluster but not push it on the git repository (CommitApply)", func() { //nolint:dupl

		ctx := context.TODO()

		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		Wait5()
		By("creating the RemoteUser & RemoteUserBinding for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyName,
				Namespace: namespace,
				Annotations: map[string]string{
					"syngit.io/associated-remote-userbinding": "true",
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

		Wait5()
		repoUrl := "http://" + gitP1Fqdn + "/syngituser/blue.git"
		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncer1Name,
				Namespace: namespace,
			},
			Spec: syngit.RemoteSyncerSpec{
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				BypassInterceptionSubjects: []v1.Subject{{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "User",
					Name:     string(Luffy),
				}},
				ExcludedFields:   []string{".metadata.uid"},
				ProcessMode:      syngit.CommitApply,
				PushMode:         syngit.SameBranch,
				RemoteRepository: repoUrl,
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
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Wait5()
		By("creating a test configmap")
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

		By("checking that the configmap is not present on the repo")
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: "syngituser",
			Name:  "blue",
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())

		By("checking that the configmap is present on the cluster")
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

	It("should apply the resource on the cluster but not push it on the git repository (CommitOnly)", func() { //nolint:dupl

		ctx := context.TODO()

		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		Wait5()
		By("creating the RemoteUser & RemoteUserBinding for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyName,
				Namespace: namespace,
				Annotations: map[string]string{
					"syngit.io/associated-remote-userbinding": "true",
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

		Wait5()
		repoUrl := "http://" + gitP1Fqdn + "/syngituser/blue.git"
		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncer2Name,
				Namespace: namespace,
			},
			Spec: syngit.RemoteSyncerSpec{
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				BypassInterceptionSubjects: []v1.Subject{{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "User",
					Name:     string(Luffy),
				}},
				ExcludedFields:   []string{".metadata.uid"},
				ProcessMode:      syngit.CommitOnly,
				PushMode:         syngit.SameBranch,
				RemoteRepository: repoUrl,
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
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Wait5()
		By("creating a test configmap")
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

		By("checking that the configmap is not present on the repo")
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: "syngituser",
			Name:  "blue",
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())

		By("checking that the configmap is present on the cluster")
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
})
