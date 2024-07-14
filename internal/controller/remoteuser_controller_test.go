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

	syngit "syngit.io/syngit/api/v2alpha2"
)

var _ = Describe("RemoteUser Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()

		const (
			timeout  = time.Second * 10
			duration = time.Second * 10
			interval = time.Millisecond * 250

			userNamespace = "default"
			resourceName  = "test-remoteuser"
		)

		const remoteGitServerName = "sample-git-server.com-conf"
		confNamespacedName := types.NamespacedName{
			Name:      remoteGitServerName,
			Namespace: userNamespace,
		}
		remoteGitServerConf := &corev1.ConfigMap{}

		defaultGitServerConfiguration := syngit.GitServerConfiguration{
			AuthenticationEndpoint: "",
			InsecureSkipTlsVerify:  false,
			CaBundle:               "",
		}

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: userNamespace,
		}
		remoteuser := &syngit.RemoteUser{}

		BeforeEach(func() {
			By("creating the custom remote git server configuration")
			err := k8sClient.Get(ctx, confNamespacedName, remoteGitServerConf)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      remoteGitServerName,
						Namespace: userNamespace,
					},
					Data: map[string]string{
						"authenticationEndpoint": "https://sample-git-server.com/api/v4/user",
						"caBundle":               "CA Bundle cert",
						"insecureSkipTlsVerify":  "true",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("Creating a RemoteUser that owns a RemoteUserBinding and is bound to a config")
			err = k8sClient.Get(ctx, typeNamespacedName, remoteuser)
			if err != nil && errors.IsNotFound(err) {
				resource := &syngit.RemoteUser{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: userNamespace,
					},
					Spec: syngit.RemoteUserSpec{
						Email:                "sample@email.com",
						GitBaseDomainFQDN:    "sample-git-server.com",
						OwnRemoteUserBinding: true,
						TestAuthentication:   false,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &syngit.RemoteUser{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			resourceConf := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, confNamespacedName, resourceConf)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance RemoteUser")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Cleanup the associated custom remote git server configuration")
			Expect(k8sClient.Delete(ctx, resourceConf)).To(Succeed())
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
				ruLookupKey := types.NamespacedName{Name: resourceName, Namespace: userNamespace}
				createRemoteUser := &syngit.RemoteUser{}

				Eventually(func() bool {
					err := k8sClient.Get(ctx, ruLookupKey, createRemoteUser)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				Expect(createRemoteUser.Status.GitServerConfiguration).Should(Equal(defaultGitServerConfiguration))

				confBool := false
				if remoteGitServerConf.Data["insecureSkipTlsVerify"] == "true" {
					confBool = true
				}
				Expect(createRemoteUser.Status.GitServerConfiguration.CaBundle).Should(Equal(remoteGitServerConf.Data["caBundle"]))
				Expect(createRemoteUser.Status.GitServerConfiguration.InsecureSkipTlsVerify).Should(Equal(confBool))
				Expect(createRemoteUser.Status.GitServerConfiguration.AuthenticationEndpoint).Should(Equal(remoteGitServerConf.Data["authenticationEndpoint"]))
			})
		})
	})
})
