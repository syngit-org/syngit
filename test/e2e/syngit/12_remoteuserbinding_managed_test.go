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
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	. "github.com/syngit-org/syngit/test/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("12 RemoteUserBinding managed by checker", func() {

	const (
		remoteUserLuffyName = "remoteuser-luffy"
	)

	It("should create a remoteuserbinding with a suffix number", func() {

		By("creating the RemoteUser")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyName,
				Namespace: namespace,
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

		By("creating the RemoteUserBinding with the exact same name as the generated one")
		remoteUserBindingLuffyName := syngit.RubPrefix + string(Luffy)
		remoteUserBindingLuffy := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBindingLuffyName,
				Namespace: namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs: []corev1.ObjectReference{
					{
						Name:      remoteUserLuffyName,
						Namespace: namespace,
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserBindingLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUserBinding with the exact same name as the generated one & the suffix 1")
		remoteUserBindingLuffyName1 := fmt.Sprintf("%s%s-1", syngit.RubPrefix, string(Luffy))
		remoteUserBindingLuffy1 := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBindingLuffyName1,
				Namespace: namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs: []corev1.ObjectReference{
					{
						Name:      remoteUserLuffyName,
						Namespace: namespace,
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserBindingLuffy1)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("updating the RemoteUser to use the automatic association")
		luffySecretName = string(Luffy) + "-creds"
		remoteUserLuffy = &syngit.RemoteUser{
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

		By("checking if the RemoteUserBinding with suffix 2 exists")
		Wait3()
		remoteUserBindingName := fmt.Sprintf("%s%s-2", syngit.RubPrefix, string(Luffy))
		nnRubLuffy := types.NamespacedName{
			Name:      remoteUserBindingName,
			Namespace: namespace,
		}
		rubLuffy := &syngit.RemoteUserBinding{}
		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnRubLuffy, rubLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})

})
