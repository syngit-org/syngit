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
	syngitv1beta1 "github.com/syngit-org/syngit/pkg/api/v1beta1"
	syngitv1beta2 "github.com/syngit-org/syngit/pkg/api/v1beta2"
	. "github.com/syngit-org/syngit/test/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("15 conversion webhook test", func() {

	const (
		remoteUserLuffyName        = "remoteuser-luffy"
		remoteUserBindingLuffyName = "remoteuserbinding-luffy"
		remoteSyncerName           = "remotesyncer-test15"
	)

	It("should convert from the previous apiversion to the current one", func() {
		By("adding syngit to scheme")
		err := syngitv1beta1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())
		err = syngitv1beta2.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		By("creating the RemoteUser for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngitv1beta1.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserLuffyName,
				Namespace: namespace,
			},
			Spec: syngitv1beta1.RemoteUserSpec{
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

		By("checking that the RemoteUser is of the current apiversion")
		nnRuLuffy := types.NamespacedName{
			Name:      remoteUserLuffyName,
			Namespace: namespace,
		}
		ruLuffy := &syngitv1beta2.RemoteUser{}
		Eventually(func() bool {
			err = sClient.As(Luffy).Get(nnRuLuffy, ruLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUserBinding for Luffy")
		remoteUserBindingLuffy := &syngitv1beta1.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBindingLuffyName,
				Namespace: namespace,
			},
			Spec: syngitv1beta1.RemoteUserBindingSpec{
				RemoteRefs: []corev1.ObjectReference{
					{
						Name:      "fake-remoteuser",
						Namespace: namespace,
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserBindingLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the RemoteUserBinding is of the current apiversion")
		nnRubLuffy := types.NamespacedName{
			Name:      remoteUserBindingLuffyName,
			Namespace: namespace,
		}
		rubLuffy := &syngitv1beta2.RemoteUserBinding{}
		Eventually(func() bool {
			err = sClient.As(Luffy).Get(nnRubLuffy, rubLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteSyncer")
		remotesyncer := &syngitv1beta1.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName,
				Namespace: namespace,
			},
			Spec: syngitv1beta1.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBlockAppliedMessage:  defaultDeniedMessage,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngitv1beta1.Block,
				ExcludedFields:              []string{".metadata.uid"},
				ProcessMode:                 syngitv1beta1.CommitOnly,
				PushMode:                    syngitv1beta1.SameBranch,
				RemoteRepository:            "https://fake-repo.com",
				ScopedResources: syngitv1beta1.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
						},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"configmaps"},
						},
					},
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer)
			fmt.Println(err)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the RemoteSyncer is of the current apiversion")
		nnRsyLuffy := types.NamespacedName{
			Name:      remoteSyncerName,
			Namespace: namespace,
		}
		rsyLuffy := &syngitv1beta2.RemoteSyncer{}
		Eventually(func() bool {
			err = sClient.As(Luffy).Get(nnRsyLuffy, rsyLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})
})
