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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta2"
	. "github.com/syngit-org/syngit/test/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("11 RemoteUserBinding permissions checker", func() {

	const (
		remoteUserBrookName        = "remoteuser-brook"
		remoteUserBindingBrookName = "remoteuserbinding-brook"
	)

	It("should deny the remoteuserbinding creation", func() {

		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		Wait5()
		By("creating the RemoteUser")
		brookSecretName := string(Brook) + "-creds"
		remoteUserBrook := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBrookName,
				Namespace: namespace,
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

		Wait5()
		By("creating the RemoteUserBinding using a not allowed remoteuser name")
		remoteUserBindingBrook := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBindingBrookName,
				Namespace: namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteRefs: []corev1.ObjectReference{
					{
						Name:      "not-allowed-remoteuser-name",
						Namespace: namespace,
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Brook).CreateOrUpdate(remoteUserBindingBrook)
			return err != nil && strings.Contains(err.Error(), rubPermissionsDeniedMessage)
		}, timeout, interval).Should(BeTrue())

	})

	It("should create the remoteuserbinding", func() {

		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		Wait5()
		By("creating the RemoteUser using an allowed secret name")
		brookSecretName := string(Brook) + "-creds"
		remoteUserBrook := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBrookName,
				Namespace: namespace,
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

		Wait5()
		By("creating the RemoteUserBinding")
		remoteUserBindingBrook := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBindingBrookName,
				Namespace: namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteRefs: []corev1.ObjectReference{
					{
						Name:      remoteUserBrookName,
						Namespace: namespace,
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Brook).CreateOrUpdate(remoteUserBindingBrook)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})

})
