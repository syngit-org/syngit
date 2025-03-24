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
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	. "github.com/syngit-org/syngit/test/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("25 Test fast-forward merge", func() {
	ctx := context.TODO()

	const (
		remoteUserLuffyName            = "remoteuser-luffy"
		remoteSyncerName1              = "remotesyncer-test25.1"
		remoteSyncerName2              = "remotesyncer-test25.2"
		remoteTargetNameCustomBranch   = "remotetarget-test25.1"
		remoteTargetNameUpstreamBranch = "remotetarget-test25.2"
		remoteUserBindingLuffyName     = "remoteuserbinding-luffy"
		cmName1                        = "test-cm25.1"
		cmName2                        = "test-cm25.2"
		cmName3                        = "test-cm25.3"
		upstreamBranch                 = "main"
		customBranch                   = "custom-branch25"
	)

	It("should correctly pull the changes from the upstream", func() {

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo1)

		By("creating the RemoteUser & RemoteUserBinding for Luffy")
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

		By("creating a RemoteTarget targeting the custom branch")
		remoteTargetCustomBranch := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteTargetNameCustomBranch,
				Namespace: namespace,
				Labels: map[string]string{
					syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
					syngit.RtLabelKeyBranch:  customBranch,
				},
			},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: repoUrl,
				TargetRepository:   repoUrl,
				UpstreamBranch:     upstreamBranch,
				TargetBranch:       customBranch,
				MergeStrategy:      syngit.TryFastForwardOrDie,
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteTargetCustomBranch)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUserBinding with the RemoteUser & RemoteTarget targeting the custom branch")
		remoteUserBindingLuffy := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBindingLuffyName,
				Namespace: namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs: []corev1.ObjectReference{
					{
						Name: remoteUserLuffyName,
					},
				},
				RemoteTargetRefs: []corev1.ObjectReference{
					{
						Name: remoteTargetNameCustomBranch,
					},
				},
				Subject: v1.Subject{
					Kind: "User",
					Name: string(Luffy),
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserBindingLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteSyncer targeting the custom-branch")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName1,
				Namespace: namespace,
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
						syngit.RtLabelKeyBranch:  customBranch,
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
		cm1 := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName1, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm1,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is present on the repo")
		Wait3()
		customBranchRepo := &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: customBranch,
		}
		exists, err := IsObjectInRepo(*customBranchRepo, cm1)
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

		By("creating a RemoteTarget targeting the upstream branch")
		remoteTargetUpstreamBranch := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteTargetNameUpstreamBranch,
				Namespace: namespace,
				Labels: map[string]string{
					syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
					syngit.RtLabelKeyBranch:  upstreamBranch,
				},
			},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: repoUrl,
				TargetRepository:   repoUrl,
				UpstreamBranch:     upstreamBranch,
				TargetBranch:       upstreamBranch,
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteTargetUpstreamBranch)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("updating the RemoteUserBinding with the RemoteUser & RemoteTarget targeting the upstream branch")
		remoteUserBindingLuffy = &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBindingLuffyName,
				Namespace: namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs: []corev1.ObjectReference{
					{
						Name: remoteUserLuffyName,
					},
				},
				RemoteTargetRefs: []corev1.ObjectReference{
					{
						Name: remoteTargetNameUpstreamBranch,
					},
					{
						Name: remoteTargetNameCustomBranch,
					},
				},
				Subject: v1.Subject{
					Kind: "User",
					Name: string(Luffy),
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserBindingLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteSyncer targeting the upstream main branch")
		remotesyncer2 := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName2,
				Namespace: namespace,
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
						syngit.RtLabelKeyBranch:  upstreamBranch,
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
			Owner:  giteaBaseNs,
			Name:   repo1,
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

		By("performing a merge from the custom-branch to the main branch")
		Eventually(func() bool {
			mergeErr := Merge(*customBranchRepo, customBranch, upstreamBranch)
			return mergeErr == nil
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
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the first custom-branch configmap is present in the custom-branch")
		Wait3()
		exists, err = IsObjectInRepo(*customBranchRepo, cm1)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

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
