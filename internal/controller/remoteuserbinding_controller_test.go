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
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	syngit "syngit.io/syngit/api/v1beta1"
)

var _ = Describe("RemoteUserBinding Controller", func() {

	const (
		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250

		userNamespace = "default"
	)

	Context("When reconciling a resource", func() {
		const (
			resourceName = "test-remoteuserbinding"
			dummySecret  = "dummy-secret"
			dummyUser    = "dummy-secret"
		)

		const remoteusername = "sample-remoteuser"
		remoteuserNamespacedName := types.NamespacedName{
			Name:      remoteusername,
			Namespace: userNamespace,
		}
		remoteUser := &syngit.RemoteUser{}

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: userNamespace,
		}
		remoteUserBinding := &syngit.RemoteUserBinding{}

		ctx := context.Background()

		BeforeEach(func() {

			By("Creating the RemoteUser")
			err := k8sClient.Get(ctx, remoteuserNamespacedName, remoteUser)
			if err != nil && errors.IsNotFound(err) {
				resource := &syngit.RemoteUser{
					ObjectMeta: metav1.ObjectMeta{
						Name:      remoteusername,
						Namespace: userNamespace,
					},
					Spec: syngit.RemoteUserSpec{
						Email:                       "sample@email.com",
						GitBaseDomainFQDN:           "sample-git-server.com",
						AssociatedRemoteUserBinding: true,
						SecretRef: corev1.SecretReference{
							Name: dummySecret,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("Creating the RemoteUserBinding")
			err = k8sClient.Get(ctx, typeNamespacedName, remoteUserBinding)
			if err != nil && errors.IsNotFound(err) {
				resource := &syngit.RemoteUserBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: userNamespace,
					},
					Spec: syngit.RemoteUserBindingSpec{
						RemoteRefs: []corev1.ObjectReference{{Name: remoteusername}},
						Subject: rbacv1.Subject{
							Kind: rbacv1.UserKind,
							Name: dummyUser,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {

			err := k8sClient.Get(ctx, remoteuserNamespacedName, remoteUser)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, typeNamespacedName, remoteUserBinding)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance RemoteUser")
			Expect(k8sClient.Delete(ctx, remoteUser)).To(Succeed())

			By("Cleanup the specific resource instance RemoteUserBinding")
			Expect(k8sClient.Delete(ctx, remoteUserBinding)).To(Succeed())
		})

		It("Should successfully reconcile the resource", func() {
			ruLookupKeyRub := types.NamespacedName{Name: resourceName, Namespace: userNamespace}
			createdRemoteUserBinding := &syngit.RemoteUserBinding{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, ruLookupKeyRub, createdRemoteUserBinding)
				return err == nil && createdRemoteUserBinding.Status.UserKubernetesID != ""
			}, timeout, interval).Should(BeTrue())

			Expect(createdRemoteUserBinding.Status.UserKubernetesID).Should(Equal(dummyUser))
		})
	})
})
