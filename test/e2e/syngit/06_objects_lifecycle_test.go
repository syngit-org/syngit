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
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("06 Test objects lifecycle", func() {

	const (
		remoteUserLuffyName        = "remoteuser-luffy-jupyter"
		remoteUserLuffyJupyterName = "remoteuser-luffy-jupyter"
		remoteUserLuffySaturnName  = "remoteuser-luffy-saturn"
		remoteSyncerName           = "remotesyncer-test6"
		branch                     = "main"
	)

	It("should properly manage the RemoteUserBinding associated webhooks", func() {
		luffySecretName := string(Luffy) + "-creds"

		By("creating the RemoteUser & RemoteUserBinding for Luffy (jupyter)")
		remoteUserLuffyJupyter := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyJupyterName,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RubAnnotationKeyManaged: "true",
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
					syngit.RubAnnotationKeyManaged: "true",
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
					syngit.RubAnnotationKeyManaged: "true",
					"change":                       "something",
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

		By("checking that the RemoteUserBinding refers to 2 RemoteUsers")
		Wait3()
		nnRub := types.NamespacedName{
			Name:      fmt.Sprintf("%s-%s", syngit.RubNamePrefix, SanitizeUsername(string(Luffy))),
			Namespace: namespace,
		}
		getRub := &syngit.RemoteUserBinding{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnRub, getRub)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Expect(getRub.Spec.RemoteUserRefs).To(HaveLen(2))

		By("deleting the saturn RemoteUser")
		Eventually(func() bool {
			err := sClient.As(Luffy).Delete(remoteUserLuffySaturn)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that RemoteUserBinding now refers to only one RemoteUser")
		Wait3()
		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnRub, getRub)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Expect(getRub.Spec.RemoteUserRefs).To(HaveLen(1))

		By("deleting the jupyter RemoteUser")
		Eventually(func() bool {
			err := sClient.As(Luffy).Delete(remoteUserLuffyJupyter)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that RemoteUserBinding does not exists")
		err := sClient.As(Luffy).Get(nnRub, getRub)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should properly manage the RemoteSyncer associated webhooks", func() {

		By("creating the RemoteUser & RemoteUserBinding for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyName,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RubAnnotationKeyManaged: "true",
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

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo1)
		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: branch,
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               branch,
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
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

		By("checking that the ValidationWebhook scopes sts")
		Wait3()
		nnValidation := types.NamespacedName{
			Name: dynamicWebhookName,
		}
		getValidation := &admissionv1.ValidatingWebhookConfiguration{}

		Eventually(func() bool {
			err := sClient.As(Admin).Get(nnValidation, getValidation)
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
