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
	"context"
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

var _ = Describe("25 Test merge strategies", func() {
	ctx := context.TODO()

	const (
		remoteUserLuffyName = "remoteuser-luffy"
		remoteSyncerName1   = "remotesyncer-test25.1"
		remoteSyncerName2   = "remotesyncer-test25.2"
		cmName1             = "test-cm25.1"
		cmName2             = "test-cm25.2"
		cmName3             = "test-cm25.3"
		upstreamBranch      = "main"
		customBranch        = "custom-branch25"
	)

	It("should correctly pull the changes from the upstream", func() {

		repoUrl := "https://" + gitP1Fqdn + "/syngituser/blue.git"

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

		By("creating the RemoteSyncer targetting the custom-branch")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName1,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationEnabled:  "true",
					syngit.RtAnnotationBranches: customBranch,
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               upstreamBranch,
				DefaultUnauthorizedUserMode: syngit.Block,
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            repoUrl,
				RemoteTargetSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
						syngit.RtLabelBranchKey:  customBranch,
					},
				},
				ScopedResources: syngit.ScopedResources{
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
		remotesyncerDeepCopied := remotesyncer.DeepCopy()
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap on the custom-branch")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName1, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			fmt.Println(err)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is present on the repo")
		Wait3()
		customBranchRepo := &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  "syngituser",
			Name:   "blue",
			Branch: customBranch,
		}
		exists, err := IsObjectInRepo(*customBranchRepo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the configmap is present on the cluster")
		nnCm := types.NamespacedName{
			Name:      cmName1,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("deleting the first RemoteSyncer")
		delErr := sClient.As(Luffy).Delete(remotesyncer)
		Expect(delErr).ToNot(HaveOccurred())

		By("creating the RemoteSyncer targetting the upstream main branch")
		remotesyncer2 := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName2,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationEnabled: "true",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               upstreamBranch,
				DefaultUnauthorizedUserMode: syngit.Block,
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            repoUrl,
				RemoteTargetSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
						syngit.RtLabelBranchKey:  upstreamBranch,
					},
				},
				ScopedResources: syngit.ScopedResources{
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
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer2)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating another test configmap on the main branch")
		Wait3()
		cm2 := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName2, Namespace: namespace},
			Data:       map[string]string{"test": "non"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm2,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is present on the repo")
		Wait3()
		upstreamRepo := &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  "syngituser",
			Name:   "blue",
			Branch: upstreamBranch,
		}
		exists, err = IsObjectInRepo(*upstreamRepo, cm2)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the configmap is present on the cluster")
		nnCm2 := types.NamespacedName{
			Name:      cmName2,
			Namespace: namespace,
		}
		getCm = &corev1.ConfigMap{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm2, getCm)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("deleting the second RemoteSyncer")
		delErr = sClient.As(Luffy).Delete(remotesyncer2)
		Expect(delErr).ToNot(HaveOccurred())

		By("re-creating the first RemoteSyncer")
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncerDeepCopied)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating another test configmap on the custom-branch")
		Wait3()
		cm3 := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName3, Namespace: namespace},
			Data:       map[string]string{"test": "non"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm3,
				metav1.CreateOptions{},
			)
			fmt.Println(err)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the previous upstream configmap is present in the custom-branch")
		Wait3()
		exists, err = IsObjectInRepo(*customBranchRepo, cm2)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the new configmap is present in the custom-branch")
		Wait3()
		exists, err = IsObjectInRepo(*customBranchRepo, cm3)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the configmap is present on the cluster")
		nnCm3 := types.NamespacedName{
			Name:      cmName3,
			Namespace: namespace,
		}
		getCm = &corev1.ConfigMap{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm3, getCm)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})
})
