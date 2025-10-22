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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	. "github.com/syngit-org/syngit/test/utils"
	"gopkg.in/yaml.v2"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("04 Create RemoteSyncer with excluded fields", func() {
	ctx := context.TODO()

	const (
		cmName1             = "test-cm4.1"
		cmName2             = "test-cm4.2"
		remoteUserLuffyName = "remoteuser-luffy"
		remoteSyncerName    = "remotesyncer-test4"
		branch              = "main"
	)

	It("should exclude the selected fields from the git repo", func() {
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

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo1)
		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: branch,
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBlockAppliedMessage:  defaultDeniedMessage,
				DefaultBranch:               branch,
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields: []string{
					".metadata.uid",
					"metadata.managedFields",
					"metadata.annotations[test-annotation1]",
					"metadata.annotations.[test-annotation2]",
				},
				Strategy:         syngit.CommitOnly,
				TargetStrategy:   syngit.OneTarget,
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

		By("creating a test configmap")
		Wait3()
		const (
			annotation1Key = "test-annotation1"
			annotation2Key = "test-annotation2"
			annotation3Key = "test-annotation3"
		)
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName1, Namespace: namespace, Annotations: map[string]string{
				annotation1Key: "test",
				annotation2Key: "test",
				annotation3Key: "test",
			}},
			Data: map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err != nil && strings.Contains(err.Error(), defaultDeniedMessage)
		}, timeout, interval).Should(BeTrue())

		By("checking if the right fields are present on the ConfigMap on the repo")
		Wait3()
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: giteaBaseNs,
			Name:  repo1,
		}
		uidExists, err := IsFieldDefined(*repo, cm, "metadata.uid")
		Expect(err).ToNot(HaveOccurred())
		Expect(uidExists).To(BeFalse())
		managedFieldsExists, err := IsFieldDefined(*repo, cm, "metadata.managedFields")
		Expect(err).ToNot(HaveOccurred())
		Expect(managedFieldsExists).To(BeFalse())

		tree, err := GetRepoTree(*repo)
		Expect(err).ToNot(HaveOccurred())
		getCm, err := GetObjectInRepo(*repo, tree, cm)
		Expect(err).ToNot(HaveOccurred())
		var parsed map[interface{}]interface{}
		err = yaml.Unmarshal(getCm.Content, &parsed)
		Expect(err).ToNot(HaveOccurred())
		metadata := parsed["metadata"].(map[interface{}]interface{})
		annotations := metadata["annotations"].(map[interface{}]interface{})
		annotation1 := annotations[annotation1Key]
		Expect(annotation1).To(BeNil())
		annotation2 := annotations[annotation2Key]
		Expect(annotation2).To(BeNil())
		annotation3 := annotations[annotation3Key]
		Expect(annotation3).To(Equal("test"))

	})

	It("should exclude the fields (configured in the ConfigMap) from the git repo", func() {
		By("creating the RemoteUser & RemoteUserBinding for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuser-luffy",
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

		const excludedFieldsConfiMapName = "excluded-fields"
		excludedFieldsConfiMap := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: excludedFieldsConfiMapName, Namespace: namespace},
			Data: map[string]string{
				"excludedFields": "[\"metadata.uid\", \"metadata.managedFields\", \"metadata.annotations[test-annotation1]\", \"metadata.annotations.[test-annotation2]\"]", //nolint:lll
			},
		}
		_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
			excludedFieldsConfiMap,
			metav1.CreateOptions{},
		)
		Expect(err).ToNot(HaveOccurred())

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo1)
		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: branch,
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBlockAppliedMessage:  defaultDeniedMessage,
				DefaultBranch:               branch,
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFieldsConfigMapRef: &corev1.ObjectReference{
					Name:      excludedFieldsConfiMapName,
					Namespace: namespace,
				},
				Strategy:         syngit.CommitOnly,
				TargetStrategy:   syngit.OneTarget,
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

		By("creating a test configmap")
		Wait3()
		const annotation1Key = "test-annotation1"
		const annotation2Key = "test-annotation2"
		const annotation3Key = "test-annotation3"
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName2, Namespace: namespace, Annotations: map[string]string{
				annotation1Key: "test",
				annotation2Key: "test",
				annotation3Key: "test",
			}},
			Data: map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err != nil && strings.Contains(err.Error(), defaultDeniedMessage)
		}, timeout, interval).Should(BeTrue())

		By("checking if the right fields are present on the ConfigMap on the repo")
		Wait3()
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: giteaBaseNs,
			Name:  repo1,
		}
		uidExists, err := IsFieldDefined(*repo, cm, "metadata.uid")
		Expect(err).ToNot(HaveOccurred())
		Expect(uidExists).To(BeFalse())
		managedFieldsExists, err := IsFieldDefined(*repo, cm, "metadata.managedFields")
		Expect(err).ToNot(HaveOccurred())
		Expect(managedFieldsExists).To(BeFalse())

		tree, err := GetRepoTree(*repo)
		Expect(err).ToNot(HaveOccurred())
		getCm, err := GetObjectInRepo(*repo, tree, cm)
		Expect(err).ToNot(HaveOccurred())
		var parsed map[interface{}]interface{}
		err = yaml.Unmarshal(getCm.Content, &parsed)
		Expect(err).ToNot(HaveOccurred())
		metadata := parsed["metadata"].(map[interface{}]interface{})
		annotations := metadata["annotations"].(map[interface{}]interface{})
		annotation1 := annotations[annotation1Key]
		Expect(annotation1).To(BeNil())
		annotation2 := annotations[annotation2Key]
		Expect(annotation2).To(BeNil())
		annotation3 := annotations[annotation3Key]
		Expect(annotation3).To(Equal("test"))
	})
})
