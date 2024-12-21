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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	syngit "github.com/syngit-org/syngit/api/v1beta2"
	. "github.com/syngit-org/syngit/test/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("01 Create RemoteUser", func() {

	const (
		remoteUserLuffyName = "remoteuser-luffy"
		remoteUserSanjiName = "remoteuser-sanji"
	)

	It("should instantiate the RemoteUser correctly (with RemoteUserBinding)", func() {
		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		Wait5()
		By("creating the RemoteUser for Luffy")
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
		nnRuLuffy := types.NamespacedName{
			Name:      fmt.Sprintf("%s%s", syngit.RubPrefix, string(Luffy)),
			Namespace: namespace,
		}
		ruLuffy := &syngit.RemoteUser{}
		_ = sClient.As(Luffy).Get(nnRuLuffy, ruLuffy)

		By("checking if the RemoteUserBinding for Luffy exists")
		nnRubLuffy := types.NamespacedName{
			Name:      fmt.Sprintf("%s%s", syngit.RubPrefix, string(Luffy)),
			Namespace: namespace,
		}
		rubLuffy := &syngit.RemoteUserBinding{}
		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnRubLuffy, rubLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Wait5()
		By("creating the RemoteUser for Sanji (without RemoteUserBinding)")
		sanjiSecretName := string(Sanji) + "-creds"
		remoteUserSanji := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserSanjiName,
				Namespace: namespace,
			},
			Spec: syngit.RemoteUserSpec{
				Email:             "sample@email.com",
				GitBaseDomainFQDN: gitP1Fqdn,
				SecretRef: corev1.SecretReference{
					Name: sanjiSecretName,
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Sanji).CreateOrUpdate(remoteUserSanji)
			return err == nil
		}, timeout, interval).Should(BeTrue())
		nnRuSanji := types.NamespacedName{
			Name:      fmt.Sprintf("%s%s", syngit.RubPrefix, string(Sanji)),
			Namespace: namespace,
		}
		ruSanji := &syngit.RemoteUser{}
		_ = sClient.As(Sanji).Get(nnRuSanji, ruSanji)

		By("checking that the RemoteUserBinding for Sanji does not exist")
		nnRubSanji := types.NamespacedName{
			Name:      fmt.Sprintf("%s%s", syngit.RubPrefix, string(Sanji)),
			Namespace: namespace,
		}
		rubSanji := &syngit.RemoteUserBinding{}

		Wait10()
		errRub := sClient.As(Sanji).Get(nnRubSanji, rubSanji)
		Expect(errRub).To(HaveOccurred())
		Expect(errRub.Error()).To(ContainSubstring("not found"))
	})
})
