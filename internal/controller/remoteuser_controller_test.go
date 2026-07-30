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

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
)

var _ = Describe("RemoteUser Controller", func() {

	const (
		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250

		userNamespace = "default"
	)

	Context("When reconciling a resource", func() {
		const resourceName = "test-remoteuser"

		const secretRefName = "sample-secret"
		secretNamespacedName := types.NamespacedName{
			Name:      secretRefName,
			Namespace: userNamespace,
		}
		secretRef := &corev1.Secret{}
		const username = "username"
		const password = "password"

		const resourceNameAssociated = resourceName + "-associated-to-rub"
		typeNamespacedNameAssociated := types.NamespacedName{
			Name:      resourceNameAssociated,
			Namespace: userNamespace,
		}
		remoteuser := &syngit.RemoteUser{}

		ctx := context.Background()

		BeforeEach(func() {

			By("Creating the secret credentials")
			err := k8sClient.Get(ctx, secretNamespacedName, secretRef)
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
					Type: "kubernetes.io/basic-auth",
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("Creating a RemoteUser that is associated to a RemoteUserBinding")
			err = k8sClient.Get(ctx, typeNamespacedNameAssociated, remoteuser)
			if err != nil && errors.IsNotFound(err) {
				resource := &syngit.RemoteUser{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameAssociated,
						Namespace: userNamespace,
						Annotations: map[string]string{
							syngit.RubAnnotationKeyManaged: "true",
						},
					},
					Spec: syngit.RemoteUserSpec{
						Email:             "sample@email.com",
						GitBaseDomainFQDN: "sample-git-server.com",
						SecretRef: corev1.SecretReference{
							Name: secretRefName,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			err := k8sClient.Get(ctx, typeNamespacedNameAssociated, remoteuser)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, secretNamespacedName, secretRef)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance RemoteUser")
			Expect(k8sClient.Delete(ctx, remoteuser)).To(Succeed())

			By("Cleanup the associated credentials secret reference")
			Expect(k8sClient.Delete(ctx, secretRef)).To(Succeed())
		})

		// It("Should create a RemoteUserBinding", func() {
		// 	createdRemoteUserBindings := &syngit.RemoteUserBindingList{}
		// 	listOps := &client.ListOptions{
		// 		Namespace: userNamespace,
		// 	}

		// 	Eventually(func() bool {
		// 		err := k8sClient.List(ctx, createdRemoteUserBindings, listOps)
		// 		return err == nil && len(createdRemoteUserBindings.Items) > 0
		// 	}, timeout, interval).Should(BeTrue())

		// 	Expect(createdRemoteUserBindings.Items[0].Spec.RemoteRefs).Should(ContainElement(map[string]corev1.ObjectReference{"name": {Name: resourceName}}))

		// })

		It("Should correctly be bound to the secret", func() {
			ruLookupKeyRu := types.NamespacedName{Name: resourceNameAssociated, Namespace: userNamespace}
			createdRemoteUser := &syngit.RemoteUser{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, ruLookupKeyRu, createdRemoteUser)
				return err == nil && createdRemoteUser.Status.SecretBoundStatus != ""
			}, timeout, interval).Should(BeTrue())

			Expect(createdRemoteUser.Status.SecretBoundStatus).Should(Equal(syngit.SecretBound))
		})

	})

	// A RemoteUser may keep its credentials in another namespace. The controller
	// must read the Secret where the reference resolves, and must stay
	// level-triggered on it: the indexer is keyed on the resolved namespace, so a
	// change to a Secret living elsewhere still re-triggers this RemoteUser.
	Context("When the secretRef points to another namespace", func() {
		const secretNamespace = "remoteuser-credentials"
		const resourceName = "test-remoteuser-cross-namespace"
		const secretRefName = "cross-namespace-secret"

		ctx := context.Background()

		remoteUserKey := types.NamespacedName{Name: resourceName, Namespace: userNamespace}
		secretKey := types.NamespacedName{Name: secretRefName, Namespace: secretNamespace}

		BeforeEach(func() {
			By("Creating the namespace that holds the credentials")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: secretNamespace}}
			err := k8sClient.Create(ctx, ns)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("Creating the secret credentials in that other namespace")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretRefName,
					Namespace: secretNamespace,
				},
				StringData: map[string]string{
					"username": "username",
					"password": "password",
				},
				Type: "kubernetes.io/basic-auth",
			}
			err = k8sClient.Create(ctx, secret)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("Creating a RemoteUser referencing the secret across namespaces")
			remoteUser := &syngit.RemoteUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: userNamespace,
				},
				Spec: syngit.RemoteUserSpec{
					Email:             "sample@email.com",
					GitBaseDomainFQDN: "sample-git-server.com",
					SecretRef: corev1.SecretReference{
						Name:      secretRefName,
						Namespace: secretNamespace,
					},
				},
			}
			err = k8sClient.Create(ctx, remoteUser)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		})

		AfterEach(func() {
			remoteUser := &syngit.RemoteUser{}
			if err := k8sClient.Get(ctx, remoteUserKey, remoteUser); err == nil {
				Expect(k8sClient.Delete(ctx, remoteUser)).To(Succeed())
			}

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, secretKey, secret); err == nil {
				Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			}
		})

		It("Should bind to the secret of the referenced namespace", func() {
			createdRemoteUser := &syngit.RemoteUser{}

			Eventually(func() syngit.SecretBoundStatus {
				if err := k8sClient.Get(ctx, remoteUserKey, createdRemoteUser); err != nil {
					return ""
				}
				return createdRemoteUser.Status.SecretBoundStatus
			}, timeout, interval).Should(Equal(syngit.SecretBound))
		})

		It("Should react to a change of that secret in the other namespace", func() {
			createdRemoteUser := &syngit.RemoteUser{}
			Eventually(func() syngit.SecretBoundStatus {
				if err := k8sClient.Get(ctx, remoteUserKey, createdRemoteUser); err != nil {
					return ""
				}
				return createdRemoteUser.Status.SecretBoundStatus
			}, timeout, interval).Should(Equal(syngit.SecretBound))

			By("Retyping the secret so that it is no longer a basic-auth one")
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, secretKey, secret)).To(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			// Only the cross-namespace watch mapping can carry this change back
			// to a RemoteUser that lives in another namespace.
			Eventually(func() syngit.SecretBoundStatus {
				if err := k8sClient.Get(ctx, remoteUserKey, createdRemoteUser); err != nil {
					return ""
				}
				return createdRemoteUser.Status.SecretBoundStatus
			}, timeout, interval).Should(Equal(syngit.SecretNotFound))
		})
	})

})
