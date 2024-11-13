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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	syngit "syngit.io/syngit/api/v1beta1"
	. "syngit.io/syngit/test/utils"
)

var _ = Describe("01 Create RemoteUsers", func() {
	ctx := context.TODO()

	const (
		timeout  = time.Second * 60
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	It("should instanciate the RemoteUsers correctly", func() {
		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		By("creating the Secret & RemoteUser for Luffy")
		luffySecretName := "luffy-jupyter"
		secretLuffyJupyter := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      luffySecretName,
				Namespace: namespace,
			},
			StringData: map[string]string{
				"username": string(Luffy),
				"password": string(Luffy) + "-pwd",
			},
			Type: "kubernetes.io/basic-auth",
		}
		_, err = client.KAs(Luffy).CoreV1().Secrets(namespace).Create(ctx,
			secretLuffyJupyter,
			metav1.CreateOptions{},
		)
		Expect(err).NotTo(HaveOccurred())

		Wait10()
		resource := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuser-luffy",
				Namespace: namespace,
			},
			Spec: syngit.RemoteUserSpec{
				Email:                       "sample@email.com",
				GitBaseDomainFQDN:           GitP1Fqdn,
				AssociatedRemoteUserBinding: true,
				SecretRef: corev1.SecretReference{
					Name: luffySecretName,
				},
			},
		}
		Eventually(func() bool {
			err := client.As(Luffy).Create(ctx, resource)
			return err == nil
		}, timeout, interval).Should(BeTrue())
		nnRuLuffy := types.NamespacedName{
			Name:      fmt.Sprintf("%s%s", syngit.RubPrefix, string(Luffy)),
			Namespace: namespace,
		}
		ruLuffy := &syngit.RemoteUser{}
		_ = client.As(Luffy).Get(ctx, nnRuLuffy, ruLuffy)

		By("checking if the RemoteUserBinding for Luffy exists")
		nnRubLuffy := types.NamespacedName{
			Name:      fmt.Sprintf("%s%s", syngit.RubPrefix, string(Luffy)),
			Namespace: namespace,
		}
		rubLuffy := &syngit.RemoteUserBinding{}
		Eventually(func() bool {
			err := client.As(Luffy).Get(ctx, nnRubLuffy, rubLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Wait5()
		repoUrl := "http://" + GitP1Fqdn + "/syngituser/blue.git"
		const defaultDeniedMessage = "DENIED ON PURPOSE"
		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test",
				Namespace: namespace,
			},
			Spec: syngit.RemoteSyncerSpec{
				DefaultBlockAppliedMessage:  defaultDeniedMessage,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				ProcessMode:                 syngit.CommitOnly,
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
		Eventually(func() bool {
			err := client.As(Luffy).Create(ctx, remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Wait5()
		By("creating a test configmap")
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		_, err = client.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
			cm,
			metav1.CreateOptions{},
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(defaultDeniedMessage))

		By("checking if the configmap is present on the repo")
		repo := &Repo{
			Fqdn:  GitP1Fqdn,
			Owner: "syngituser",
			Name:  "blue",
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())
	})

	// Context("Test File Existence", func() {
	// 	It("should check if the file exists", func() {
	// 		repoOwner := "user1"
	// 		repoName := "example-repo"
	// 		filePath := "src/main.go"
	// 		tree, err := utils.GetRepoTree(repoOwner, repoName, "root_sha_of_repo")
	// 		Expect(err).NotTo(HaveOccurred())

	// 		// Check if the file exists
	// 		exists := utils.IsFilePresent(tree, filePath)
	// 		Expect(exists).To(BeTrue(), fmt.Sprintf("File '%s' should exist", filePath))
	// 	})
	// })

	// Add more custom tests as needed...
})

func Wait5() {
	time.Sleep(5 * time.Second)
}

func Wait10() {
	time.Sleep(10 * time.Second)
}
