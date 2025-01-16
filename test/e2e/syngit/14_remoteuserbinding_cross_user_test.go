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
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	. "github.com/syngit-org/syngit/test/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("14 RemoteUser RBAC cross user test", func() {

	const (
		remoteUserLuffyName   = "remoteuser-luffy"
		remoteUserChopperName = "remoteuser-chopper"
	)

	It("should deny update of the RemoteUser", func() {
		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		By("creating the RemoteUser for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyName,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RubAnnotation: "true",
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

		By("creating the RemoteUser for Chopper")
		chopperSecretName := string(Chopper) + "-creds"
		remoteUserChopper := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserChopperName,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RubAnnotation: "true",
				},
			},
			Spec: syngit.RemoteUserSpec{
				Email:             "sample@email.com",
				GitBaseDomainFQDN: gitP1Fqdn,
				SecretRef: corev1.SecretReference{
					Name: chopperSecretName,
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Chopper).CreateOrUpdate(remoteUserChopper)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("modifying removing Chopper rub annotation using Luffy")
		Wait3()
		remoteUserChopperWithNoAssociatedRub := remoteUserChopper.DeepCopy()
		remoteUserChopperWithNoAssociatedRub.Annotations = map[string]string{
			syngit.RubAnnotation: "false",
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserChopperWithNoAssociatedRub)
			return err != nil && strings.Contains(err.Error(), crossRubErrorMessage)
		}, timeout, interval).Should(BeTrue())

		By("modifying removing Chopper rub annotation using Chopper")
		Wait3()
		Eventually(func() bool {
			err := sClient.As(Chopper).CreateOrUpdate(remoteUserChopperWithNoAssociatedRub)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})
})
