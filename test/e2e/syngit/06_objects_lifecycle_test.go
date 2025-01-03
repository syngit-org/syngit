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
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta2"
	. "github.com/syngit-org/syngit/test/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("06 Test objects lifecycle", func() {

	const (
		remoteUserLuffyName               = "remoteuser-luffy"
		remoteUserLuffyJupyterName        = "remoteuser-luffy"
		remoteUserLuffySaturnName         = "remoteuser-luffy-saturn"
		remotesyncerValidationWebhookName = "remotesyncer.syngit.io"
		remoteSyncerName                  = "remotesyncer-test6"
	)

	It("should properly manage the RemoteUserBinding associated webhooks", func() {
		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		luffySecretName := string(Luffy) + "-creds"

		Wait5()
		By("creating the RemoteUser & RemoteUserBinding for Luffy (jupyter)")
		remoteUserLuffyJupyter := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyJupyterName,
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
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserLuffyJupyter)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUser & RemoteUserBinding for Luffy (saturn)")
		remoteUserLuffySaturn := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffySaturnName,
				Namespace: namespace,
				Annotations: map[string]string{
					"syngit.io/associated-remote-userbinding": "true",
				},
			},
			Spec: syngit.RemoteUserSpec{
				Email:             "sample@email.com",
				GitBaseDomainFQDN: gitP2Fqdn,
				SecretRef: corev1.SecretReference{
					Name: luffySecretName,
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserLuffySaturn)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("updating the RemoteUser & RemoteUserBinding for Luffy (saturn)")
		remoteUserLuffySaturn2 := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffySaturnName,
				Namespace: namespace,
				Annotations: map[string]string{
					"syngit.io/associated-remote-userbinding": "true",
					"change": "something",
				},
			},
			Spec: syngit.RemoteUserSpec{
				Email:             "sample@email.com",
				GitBaseDomainFQDN: gitP2Fqdn,
				SecretRef: corev1.SecretReference{
					Name: luffySecretName,
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserLuffySaturn2)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Wait10()
		By("checking that the RemoteUserBinding refers to 2 RemoteUsers")
		nnRub := types.NamespacedName{
			Name:      fmt.Sprintf("%s%s", syngit.RubPrefix, string(Luffy)),
			Namespace: namespace,
		}
		getRub := &syngit.RemoteUserBinding{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnRub, getRub)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Expect(len(getRub.Spec.RemoteRefs)).To(Equal(2))

		By("deleting the saturn RemoteUser")
		Eventually(func() bool {
			err := sClient.As(Luffy).Delete(remoteUserLuffySaturn)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Wait5()
		By("checking that RemoteUserBinding now refers to only one RemoteUser")
		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnRub, getRub)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Expect(len(getRub.Spec.RemoteRefs)).To(Equal(1))

		By("deleting the jupyter RemoteUser")
		Eventually(func() bool {
			err := sClient.As(Luffy).Delete(remoteUserLuffyJupyter)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Wait10()
		By("checking that RemoteUserBinding does not exists")
		err = sClient.As(Luffy).Get(nnRub, getRub)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should properly manage the RemoteSyncer associated webhooks", func() {
		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		Wait5()
		By("creating the RemoteUser & RemoteUserBinding for Luffy")
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

		Wait5()
		repoUrl := "http://" + gitP1Fqdn + "/syngituser/blue.git"
		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName,
				Namespace: namespace,
			},
			Spec: syngit.RemoteSyncerSpec{
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				ProcessMode:                 syngit.CommitApply,
				PushMode:                    syngit.SameBranch,
				RemoteRepository:            repoUrl,
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
						},
						Rule: admissionv1.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"statefulsets"},
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

		Wait5()
		By("checking that the ValidationWebhhok scopes sts")
		nnValidation := types.NamespacedName{
			Name: remotesyncerValidationWebhookName,
		}
		getValidation := &admissionv1.ValidatingWebhookConfiguration{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnValidation, getValidation)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		validations := getValidation.Webhooks
		stsNb := 0
		for _, validation := range validations {
			rule := validation.Rules[0]
			if rule.Resources[0] == remotesyncer.Spec.ScopedResources.Rules[0].Resources[0] {
				stsNb += 1
			}
		}
		Expect(stsNb).To(Equal(1))

	})
})
