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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("16 Wrong reference or value test", func() {
	ctx := context.TODO()

	const (
		remoteSyncerName           = "remotesyncer-test16"
		remoteUserLuffyName        = "remoteuser-luffy"
		remoteUserChopperName      = "remoteuser-chopper"
		remoteUserBindingLuffyName = "remoteuserbinding-luffy"
		cmName                     = "test-cm16"
		branch                     = "main"
	)

	It("should get errored because of wrong resource reference or wrong value", func() {
		By("creating the RemoteUser for Luffy with wrong secret reference")
		luffySecretName := "fake-secret"
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

		By("checking that the RemoteUser secret status is not bound")
		nnRuLuffy := types.NamespacedName{
			Name:      remoteUserLuffyName,
			Namespace: namespace,
		}
		ruLuffy := &syngit.RemoteUser{}
		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnRuLuffy, ruLuffy)
			return err == nil &&
				len(ruLuffy.Status.Conditions) > 0 &&
				ruLuffy.Status.Conditions[0].Type == "SecretBound" &&
				ruLuffy.Status.Conditions[0].Status == metav1.ConditionFalse
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteUserBinding for Luffy with wrong RemoteUser reference")
		remoteUserBindingLuffy := &syngit.RemoteUserBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserBindingLuffyName,
				Namespace: namespace,
			},
			Spec: syngit.RemoteUserBindingSpec{
				RemoteUserRefs: []corev1.ObjectReference{
					{
						Name: "fake-remoteuser",
					},
				},
				RemoteTargetRefs: []corev1.ObjectReference{
					{
						Name: "fake-remotetarget",
					},
				},
			},
		}
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserBindingLuffy)
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
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err != nil && utils.ErrorTypeChecker(&utils.RemoteUserBindingNotFoundError{}, err.Error())
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is not present on the repo")
		Wait3()
		repo := &Repo{
			Fqdn:  gitP1Fqdn,
			Owner: giteaBaseNs,
			Name:  repo1,
		}
		exists, err := IsObjectInRepo(*repo, cm)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())

		By("checking that the configmap is not present on the cluster")
		nnCm := types.NamespacedName{
			Name:      cmName,
			Namespace: namespace,
		}
		getCm := &corev1.ConfigMap{}
		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err != nil && strings.Contains(err.Error(), notPresentOnCluser)
		}, timeout, interval).Should(BeTrue())

		By("creating the RemoteSyncer using wrong referenced default user")
		remotesyncer = &syngit.RemoteSyncer{
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
				DefaultUnauthorizedUserMode: syngit.UseDefaultUser,
				DefaultRemoteUserRef: &corev1.ObjectReference{
					Name: "fake-defaultuser",
				},
				DefaultRemoteTargetRef: &corev1.ObjectReference{
					Name: "fake-defaulttarget",
				},
				ExcludedFields:   []string{".metadata.uid"},
				Strategy:         syngit.CommitApply,
				TargetStrategy:   syngit.OneTarget,
				RemoteRepository: repoUrl,
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
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap")
		Wait3()
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err != nil && utils.ErrorTypeChecker(&utils.DefaultRemoteUserNotFoundError{}, err.Error())
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is not present on the repo")
		Wait3()
		exists, err = IsObjectInRepo(*repo, cm)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())

		By("checking that the configmap is not present on the cluster")
		getCm = &corev1.ConfigMap{}
		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err != nil && strings.Contains(err.Error(), notPresentOnCluser)
		}, timeout, interval).Should(BeTrue())

		By("only creating the RemoteUser for Chopper")
		chopperSecretName := string(Chopper) + "-creds"
		remoteUserChopper := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteUserChopperName,
				Namespace: namespace,
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

		By("creating the RemoteSyncer using wrong referenced default target")
		remotesyncer = &syngit.RemoteSyncer{
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
				DefaultUnauthorizedUserMode: syngit.UseDefaultUser,
				DefaultRemoteUserRef: &corev1.ObjectReference{
					Name: remoteUserChopperName,
				},
				DefaultRemoteTargetRef: &corev1.ObjectReference{
					Name: "fake-defaulttarget",
				},
				ExcludedFields:   []string{".metadata.uid"},
				Strategy:         syngit.CommitApply,
				TargetStrategy:   syngit.OneTarget,
				RemoteRepository: repoUrl,
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
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap")
		Wait3()
		Eventually(func() bool {
			_, err = sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm,
				metav1.CreateOptions{},
			)
			return err != nil && utils.ErrorTypeChecker(&utils.DefaultRemoteTargetNotFoundError{}, err.Error())
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is not present on the repo")
		Wait3()
		exists, err = IsObjectInRepo(*repo, cm)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())

		By("checking that the configmap is not present on the cluster")
		getCm = &corev1.ConfigMap{}
		Eventually(func() bool {
			err := sClient.As(Luffy).Get(nnCm, getCm)
			return err != nil && strings.Contains(err.Error(), notPresentOnCluser)
		}, timeout, interval).Should(BeTrue())

	})
})
