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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	syngit "syngit.io/syngit/api/v1alpha4"
)

var _ = Describe("RemoteUser Controller", func() {

	const (
		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250

		userNamespace = "default"
		resourceName  = "test-remoteuser"
	)

	const remoteGitServerConfName = "sample-git-server.com-conf"
	confNamespacedName := types.NamespacedName{
		Name:      remoteGitServerConfName,
		Namespace: userNamespace,
	}
	remoteGitServerConf := &corev1.ConfigMap{}

	const secretRefName = "sample-secret"
	secretNamespacedName := types.NamespacedName{
		Name:      secretRefName,
		Namespace: userNamespace,
	}
	secretRef := &corev1.Secret{}
	const username = "username"
	const password = "password"

	defaultGitServerConfiguration := syngit.GitServerConfiguration{
		AuthenticationEndpoint: "",
		InsecureSkipTlsVerify:  false,
		CaBundle:               "",
	}
	customGitServerConfigiguration := syngit.GitServerConfiguration{
		AuthenticationEndpoint: "https://sample-git-server.com/api/v4/user",
		InsecureSkipTlsVerify:  false,
		CaBundle:               "CA Bundle cert",
	}
	const actualInsecureSkipTlsVerify = true

	const resourceNameOwned = resourceName + "-owned-by-rub"
	typeNamespacedNameOwned := types.NamespacedName{
		Name:      resourceNameOwned,
		Namespace: userNamespace,
	}
	const resourceNameCustomGit = resourceName + "-custom-git-conf"
	typeNamespacedNameCustomGit := types.NamespacedName{
		Name:      resourceNameCustomGit,
		Namespace: userNamespace,
	}

	Context("When reconciling a resource", func() {
		ctx := context.Background()

		BeforeEach(func() {
			By("Creating the custom remote git server configuration")
			err := k8sClient.Get(ctx, confNamespacedName, remoteGitServerConf)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      remoteGitServerConfName,
						Namespace: userNamespace,
					},
					Data: map[string]string{
						"authenticationEndpoint": customGitServerConfigiguration.AuthenticationEndpoint,
						"caBundle":               customGitServerConfigiguration.CaBundle,
						"insecureSkipTlsVerify": func() string {
							if customGitServerConfigiguration.InsecureSkipTlsVerify {
								return "true"
							} else {
								return "false"
							}
						}(),
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("Creating the secret credentials")
			err = k8sClient.Get(ctx, secretNamespacedName, secretRef)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretRefName,
						Namespace: userNamespace,
					},
					StringData: map[string]string{
						"username": username,
						"password": password,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			remoteuser := &syngit.RemoteUser{}

			By("Creating a RemoteUser that owns a RemoteUserBinding and is bound to a config")
			err = k8sClient.Get(ctx, typeNamespacedNameOwned, remoteuser)
			if err != nil && errors.IsNotFound(err) {
				resource := &syngit.RemoteUser{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameOwned,
						Namespace: userNamespace,
					},
					Spec: syngit.RemoteUserSpec{
						Email:                "sample@email.com",
						GitBaseDomainFQDN:    "sample-git-server.com",
						OwnRemoteUserBinding: true,
						TestAuthentication:   false,
						SecretRef: corev1.SecretReference{
							Name: secretRefName,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("Creating a RemoteUser that inherit from a remote git sever configuration")
			err = k8sClient.Get(ctx, typeNamespacedNameCustomGit, remoteuser)
			if err != nil && errors.IsNotFound(err) {
				resource := &syngit.RemoteUser{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameCustomGit,
						Namespace: userNamespace,
					},
					Spec: syngit.RemoteUserSpec{
						Email:                 "sample@email.com",
						GitBaseDomainFQDN:     "sample-git-server.com",
						OwnRemoteUserBinding:  false,
						TestAuthentication:    false,
						InsecureSkipTlsVerify: actualInsecureSkipTlsVerify,
						CustomGitServerConfigRef: corev1.ObjectReference{
							Name: remoteGitServerConfName,
						},
						SecretRef: corev1.SecretReference{
							Name: secretRefName,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resourceOwned := &syngit.RemoteUser{}
			err := k8sClient.Get(ctx, typeNamespacedNameOwned, resourceOwned)
			Expect(err).NotTo(HaveOccurred())

			resourceGitConfig := &syngit.RemoteUser{}
			err = k8sClient.Get(ctx, typeNamespacedNameCustomGit, resourceGitConfig)
			Expect(err).NotTo(HaveOccurred())

			resourceConf := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, confNamespacedName, resourceConf)
			Expect(err).NotTo(HaveOccurred())

			resourceSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, secretNamespacedName, resourceSecret)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance RemoteUser")
			Expect(k8sClient.Delete(ctx, resourceOwned)).To(Succeed())

			By("Cleanup the associated custom remote git server configuration")
			Expect(k8sClient.Delete(ctx, resourceConf)).To(Succeed())

			By("Cleanup the associated credentials secret reference")
			Expect(k8sClient.Delete(ctx, resourceSecret)).To(Succeed())
		})

		Context("When updating a RemoteUser", func() {

			// It("Should create a RemoteUserBinding", func() {
			// 	rubLookupKey := types.NamespacedName{Name: "owned-rub-kubernetes-admin", Namespace: userNamespace}
			// 	createRemoteUserBinding := &syngit.RemoteUserBinding{}

			// 	Eventually(func() bool {
			// 		err := k8sClient.Get(ctx, rubLookupKey, createRemoteUserBinding)
			// 		return err == nil
			// 	}, timeout, interval).Should(BeTrue())

			// 	Expect(createRemoteUserBinding.Spec.RemoteRefs).Should(ContainElement(corev1.ObjectReference{Name: resourceName}))

			// })
			It("Should have the configuration stored in the status", func() {
				ruLookupKey := types.NamespacedName{Name: resourceNameOwned, Namespace: userNamespace}
				createRemoteUser := &syngit.RemoteUser{}

				Eventually(func() bool {
					err := k8sClient.Get(ctx, ruLookupKey, createRemoteUser)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				Expect(createRemoteUser.Status.GitServerConfiguration).Should(Equal(defaultGitServerConfiguration))

				Expect(createRemoteUser.Status.GitServerConfiguration.CaBundle).Should(Equal(defaultGitServerConfiguration.CaBundle))
				Expect(createRemoteUser.Status.GitServerConfiguration.InsecureSkipTlsVerify).Should(Equal(defaultGitServerConfiguration.InsecureSkipTlsVerify))
				Expect(createRemoteUser.Status.GitServerConfiguration.AuthenticationEndpoint).Should(Equal((defaultGitServerConfiguration.AuthenticationEndpoint)))
			})
		})

		Context("When updating a RemoteUser", func() {

			It("Should have the top configuration propagated", func() {
				ruLookupKey := types.NamespacedName{Name: resourceNameCustomGit, Namespace: userNamespace}
				createRemoteUser := &syngit.RemoteUser{}

				Eventually(func() bool {
					err := k8sClient.Get(ctx, ruLookupKey, createRemoteUser)
					return err == nil && createRemoteUser.Status.GitServerConfiguration.CaBundle != ""
				}, timeout, interval).Should(BeTrue())

				Expect(createRemoteUser.Status.GitServerConfiguration.CaBundle).Should(Equal(customGitServerConfigiguration.CaBundle))
				Expect(createRemoteUser.Status.GitServerConfiguration.InsecureSkipTlsVerify).Should(Equal(actualInsecureSkipTlsVerify))
				Expect(createRemoteUser.Status.GitServerConfiguration.AuthenticationEndpoint).Should(Equal(customGitServerConfigiguration.AuthenticationEndpoint))
			})
		})
	})

})
