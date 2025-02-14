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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	"github.com/syngit-org/syngit/pkg/utils"
	. "github.com/syngit-org/syngit/test/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("23 RemoteTarget selector in RemoteSyncer", func() {
	ctx := context.TODO()

	const (
		remoteSyncerName1          = "remotesyncer-test23.1"
		remoteSyncerName2          = "remotesyncer-test23.2"
		remoteSyncerName3          = "remotesyncer-test23.3"
		remoteSyncerName4          = "remotesyncer-test23.4"
		remoteSyncerName5          = "remotesyncer-test23.5"
		remoteTargetName1          = "remotetarget-test23.1"
		remoteTargetName2          = "remotetarget-test23.2"
		remoteTargetName3          = "remotetarget-test23.3"
		remoteTargetName41         = "remotetarget-test23.41"
		remoteTargetName42         = "remotetarget-test23.42"
		remoteTargetName5          = "remotetarget-test23.5"
		remoteUserLuffyName        = "remoteuser-luffy"
		remoteUserBindingLuffyName = "remoteuserbinding-luffy"
		cmName1                    = "test-cm23.1"
		cmName2                    = "test-cm23.2"
		cmName3                    = "test-cm23.3"
		cmName4                    = "test-cm23.4"
		cmName5                    = "test-cm23.5"
		branch                     = "main"
	)

	It("should not work because RemoteTarget not targeted", func() {
		By("creating the RemoteUser for Luffy")
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

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo1)
		By("creating a RemoteTarget")
		const (
			myLabelKey   = "my-label-key"
			myLabelValue = "my-label-value"
		)
		remoteTarget := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteTargetName1,
				Namespace: namespace,
				Labels:    map[string]string{myLabelKey: myLabelValue},
			},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: repoUrl,
				TargetRepository:   repoUrl,
				UpstreamBranch:     branch,
				TargetBranch:       branch,
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteTarget)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUserBinding with the RemoteUser & RemoteTarget")
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
						Name: remoteTargetName1,
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

		By("creating the RemoteSyncer")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName1,
				Namespace: namespace,
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               branch,
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            repoUrl,
				RemoteTargetSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{myLabelKey: "another-value"},
				},
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
							admissionv1.Delete,
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
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName1, Namespace: namespace},
			Data:       map[string]string{"test": "ouiiii"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err != nil && utils.ErrorTypeChecker(&utils.RemoteTargetNotFoundError{}, err.Error())
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is not present on the repo")
		Wait3()
		repo := &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch,
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())

		By("checking that the configmap is not present on the cluster")
		nnCm := types.NamespacedName{
			Name:      cmName1,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}
		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err != nil && strings.Contains(err.Error(), notPresentOnCluser)
		}, timeout, interval).Should(BeTrue())

	})

	It("should work because RemoteTarget is targeted", func() {
		By("creating the RemoteUser for Luffy")
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

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo1)
		By("creating a RemoteTarget")
		const (
			myLabelKey   = "my-label-key"
			myLabelValue = "my-label-value"
		)
		remoteTarget := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteTargetName2,
				Namespace: namespace,
				Labels:    map[string]string{myLabelKey: myLabelValue},
			},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: repoUrl,
				TargetRepository:   repoUrl,
				UpstreamBranch:     branch,
				TargetBranch:       branch,
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteTarget)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUserBinding with the RemoteUser & RemoteTarget")
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
						Name: remoteTargetName2,
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

		By("creating the RemoteSyncer")
		remotesyncer2 := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName2,
				Namespace: namespace,
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               branch,
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            repoUrl,
				RemoteTargetSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{myLabelKey: myLabelValue},
				},
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
							admissionv1.Delete,
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

		By("creating a test configmap")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName2, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is present on the repo")
		Wait3()
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: giteaBaseNs,
			Name:  repo1,
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the configmap is present on the cluster")
		nnCm := types.NamespacedName{
			Name:      cmName2,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})

	It("should work because RemoteTarget selector not specified", func() {
		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		By("creating the RemoteUser for Luffy")
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

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo1)
		By("creating a RemoteTarget")
		const (
			myLabelKey   = "my-label-key"
			myLabelValue = "my-label-value"
		)
		remoteTarget := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteTargetName3,
				Namespace: namespace,
				Labels:    map[string]string{myLabelKey: myLabelValue},
			},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: repoUrl,
				TargetRepository:   repoUrl,
				UpstreamBranch:     branch,
				TargetBranch:       branch,
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteTarget)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUserBinding with the RemoteUser & RemoteTarget")
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
						Name: remoteTargetName3,
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

		By("creating the RemoteSyncer")
		remotesyncer3 := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName3,
				Namespace: namespace,
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
							admissionv1.Delete,
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
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer3)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName3, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking if the configmap is present on the repo")
		Wait3()
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: giteaBaseNs,
			Name:  repo1,
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the configmap is present on the cluster")
		nnCm := types.NamespacedName{
			Name:      cmName3,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})

	It("should work with multiple RemoteTarget", func() {
		By("adding syngit to scheme")
		err := syngit.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		By("creating the RemoteUser for Luffy")
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

		repoUrl := fmt.Sprintf("https://%s/%s/%s.git", gitP1Fqdn, giteaBaseNs, repo1)
		By("creating the first RemoteTarget")
		const (
			myLabelKey   = "my-label-key"
			myLabelValue = "my-label-value"
			branch1      = "branch23-31"
			branch2      = "branch23-32"
		)
		remoteTarget41 := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteTargetName41,
				Namespace: namespace,
				Labels:    map[string]string{myLabelKey: myLabelValue},
			},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: repoUrl,
				TargetRepository:   repoUrl,
				UpstreamBranch:     branch,
				TargetBranch:       branch1,
				MergeStrategy:      syngit.TryFastForwardOrDie,
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteTarget41)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the second RemoteTarget")
		remoteTarget42 := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteTargetName42,
				Namespace: namespace,
				Labels:    map[string]string{myLabelKey: myLabelValue},
			},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: repoUrl,
				TargetRepository:   repoUrl,
				UpstreamBranch:     branch,
				TargetBranch:       branch2,
				MergeStrategy:      syngit.TryFastForwardOrDie,
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteTarget42)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUserBinding with the RemoteUser & RemoteTarget")
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
						Name: remoteTargetName41,
					},
					{
						Name: remoteTargetName42,
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

		By("creating the RemoteSyncer")
		remotesyncer4 := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName4,
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
				TargetStrategy:              syngit.MultipleTarget,
				RemoteTargetSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{myLabelKey: myLabelValue},
				},
				RemoteRepository: repoUrl,
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
							admissionv1.Delete,
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
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer4)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName4, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking if the configmap is present on the branches")
		Wait3()
		repo := &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch1,
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())
		repo = &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch2,
		}
		exists, err = IsObjectInRepo(*repo, cm)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("checking that the configmap is present on the cluster")
		nnCm := types.NamespacedName{
			Name:      cmName4,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}

		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err == nil
		}, timeout, interval).Should(BeTrue())

	})

	It("should not work because RemoteTarget is not part of the RemoteUserBinding", func() {
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
		By("creating a RemoteTarget")
		const (
			myLabelKey   = "my-label-key"
			myLabelValue = "my-label-value"
		)
		remoteTarget := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteTargetName5,
				Namespace: namespace,
				Labels:    map[string]string{myLabelKey: myLabelValue},
			},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: repoUrl,
				TargetRepository:   repoUrl,
				UpstreamBranch:     branch,
				TargetBranch:       branch,
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteTarget)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteSyncer")
		remotesyncer5 := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName5,
				Namespace: namespace,
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               branch,
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields:              []string{".metadata.uid"},
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            repoUrl,
				RemoteTargetSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{myLabelKey: myLabelValue},
				},
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
							admissionv1.Delete,
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
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer5)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap")
		Wait3()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName5, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err != nil && utils.ErrorTypeChecker(&utils.RemoteTargetNotFoundError{}, err.Error())
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is not present on the repo")
		Wait3()
		repo := &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch,
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())

		By("checking that the configmap is not present on the cluster")
		nnCm := types.NamespacedName{
			Name:      cmName1,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}
		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err != nil && strings.Contains(err.Error(), notPresentOnCluser)
		}, timeout, interval).Should(BeTrue())

	})
})
