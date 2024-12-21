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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	syngit "github.com/syngit-org/syngit/api/v1beta2"
	. "github.com/syngit-org/syngit/test/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("09 Multi RemoteSyncer test", func() {

	const (
		remoteUserLuffyName = "remoteuser-luffy"
		remoteSyncer1Name   = "remotesyncer-test9.1"
		remoteSyncer2Name   = "remotesyncer-test9.2"
		cmName1             = "test-cm9.1"
		cmName2             = "test-cm9.2"
	)
	ctx := context.TODO()

	It("should push the ConfigMap on 2 different repo", func() {

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
		blueUrl := "http://" + gitP1Fqdn + "/syngituser/blue.git"
		By("creating the RemoteSyncer for the blue repo")
		blueRemotesyncer := &syngit.RemoteSyncer{
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
				RemoteRepository:            blueUrl,
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
		err = sClient.As(Luffy).CreateOrUpdate(blueRemotesyncer)
		Expect(err).ToNot(HaveOccurred())

		greenUrl := "http://" + gitP1Fqdn + "/syngituser/green.git"
		By("creating the RemoteSyncer for the green repo")
		greenRemotesyncer := &syngit.RemoteSyncer{
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
				RemoteRepository:            greenUrl,
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
		err = sClient.As(Luffy).CreateOrUpdate(greenRemotesyncer)
		Expect(err).ToNot(HaveOccurred())

		Wait5()
		By("creating a test configmap")
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName1, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Wait5()
		By("checking that the configmap is present in the blue repo")
		blueRepo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: "syngituser",
			Name:  "blue",
		}
		exists, err := IsObjectInRepo(*blueRepo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the configmap is present in the green repo")
		greenRepo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: "syngituser",
			Name:  "green",
		}
		exists, err = IsObjectInRepo(*greenRepo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the configmap is present on the cluster")
		nnCm := types.NamespacedName{
			Name:      cmName1,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})

	It("should push the resource & deny the request (because of locking resource)", func() {

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
		blueUrl := "http://" + gitP1Fqdn + "/syngituser/blue.git"
		By("creating the RemoteSyncer for the blue repo")
		blueRemotesyncer := &syngit.RemoteSyncer{
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
				RemoteRepository:            blueUrl,
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
		err = sClient.As(Luffy).CreateOrUpdate(blueRemotesyncer)
		Expect(err).ToNot(HaveOccurred())

		By("creating the RemoteSyncer for the blue repo twice")
		blueRemotesyncer2 := &syngit.RemoteSyncer{
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
				RemoteRepository:            blueUrl,
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
		err = sClient.As(Luffy).CreateOrUpdate(blueRemotesyncer2)
		Expect(err).ToNot(HaveOccurred())

		Wait5()
		By("creating a test configmap")
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
			return err != nil && strings.Contains(err.Error(), "cannot lock ref")
		}, timeout, interval).Should(BeTrue())

		Wait5()
		By("checking that the configmap is not present in the blue repo")
		blueRepo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: "syngituser",
			Name:  "blue",
		}
		exists, err := IsObjectInRepo(*blueRepo, cm)
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
			return err != nil
		}, timeout, interval).Should(BeTrue())

	})

})
