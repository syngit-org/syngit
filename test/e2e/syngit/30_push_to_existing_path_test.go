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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	. "github.com/syngit-org/syngit/test/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("30 Push to an existing resource", func() {
	ctx := context.TODO()

	const (
		remoteSyncerName1   = "remotesyncer-test30.1"
		remoteSyncerName2   = "remotesyncer-test30.2"
		remoteUserLuffyName = "remoteuser-luffy"
		cmName1             = "test-cm30.1"
		cmName2             = "test-cm30.2"
		branch              = "main"
	)

	It("should commit & push the resource on the path where it already exists", func() {

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
				Name:      remoteSyncerName1,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: branch,
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
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
							admissionv1.Delete,
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

		By("Creating the configmap in the repository in a custom path")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName1, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}

		repo := Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch,
		}
		err := CommitObjectOnSpecifiedPath(repo, cm, "custom-path/custom.yaml")
		Expect(err).ToNot(HaveOccurred())

		By("changing the configmap data")
		cm.Data["another"] = "test"

		By("creating the test configmap on the cluster")
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking if the configmap is present on the repo under the right path")
		Wait3()
		files, err := SearchForObjectInRepo(repo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(files).To(HaveLen(1))
		Expect(files[0].Path).To(Equal("custom.yaml"))

	})

	It("should commit & push the resource on the documents where it already exists", func() {

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
				Name:      remoteSyncerName2,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: branch,
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
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
							admissionv1.Delete,
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

		By("Creating the configmap in the repository in a custom path")
		Wait3()
		yaml := `
apiVersion: v1
kind: Pod
metadata:
  name: mypod
  namespace: test
---
data:
  test: oui
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm30.2
  namespace: test
`
		repo := Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch,
		}
		err := CommitYamlOnSpecifiedPath(repo, []byte(yaml), "custom-path/custom.yaml")
		Expect(err).ToNot(HaveOccurred())

		By("creating the test configmap on the cluster with modified data")
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName2, Namespace: namespace},
			Data:       map[string]string{"test": "non"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking if the configmap is present on the repo under the right path")
		Wait3()
		files, err := SearchForObjectInRepo(repo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(files).To(HaveLen(1))
		Expect(files[0].Path).To(Equal("custom.yaml"))

	})

})
