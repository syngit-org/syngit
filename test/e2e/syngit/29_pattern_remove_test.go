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
)

var _ = Describe("29 Add & remove patterns tests", func() {
	ctx := context.TODO()

	const (
		remoteUserLuffyName = "remoteuser-luffy"
		remoteSyncerName1   = "remotesyncer-test29.2"
		remoteSyncerName2   = "remotesyncer-test29.2"
		upstreamBranch      = "main"
		branch1             = "branch29.1"
		branch2             = "branch29.2"
		cmName1             = "test-cm29.1"
		cmName2             = "test-cm29.2"
		cmName3             = "test-cm29.3"
		cmName4             = "test-cm29.4"
		cmName5             = "test-cm29.5"
	)

	It("push on the right branches at the right moment", func() {

		By("creating the RemoteUser & RemoteUserBinding for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuser-luffy",
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
		branches := strings.Join([]string{branch1, branch2}, ", ")
		By("creating the RemoteSyncer that target everybranches")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName1,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: branches,
					syngit.RtAnnotationKeyUserSpecific:      string(syngit.RtAnnotationValueOneUserOneBranch),
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               upstreamBranch,
				DefaultUnauthorizedUserMode: syngit.Block,
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.MultipleTarget,
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

		By("creating a test configmap that should be push everywhere")
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

		By("checking that the configmap is present on the branches")
		Wait3()
		repo := &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch1,
		}
		exists, err := IsObjectInRepo(*repo, cm1)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())
		Wait3()
		repo = &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch2,
		}
		exists, err = IsObjectInRepo(*repo, cm1)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())
		Wait3()
		repo = &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: string(Luffy),
		}
		exists, err = IsObjectInRepo(*repo, cm1)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("updating the RemoteSyncer to have only user specific pattern")
		remotesyncer.Annotations[syngit.RtAnnotationKeyOneOrManyBranches] = ""
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap that should be push only on the user specific branch")
		Wait3()
		cm2 := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName2, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm2,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is present on the user branch and not on the other")
		Wait3()
		repo = &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch1,
		}
		exists, err = IsObjectInRepo(*repo, cm2)
		Expect(exists).To(BeFalse())
		Expect(err).To(HaveOccurred())
		Wait3()
		repo = &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch2,
		}
		exists, err = IsObjectInRepo(*repo, cm2)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())
		Wait3()
		repo = &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: string(Luffy),
		}
		exists, err = IsObjectInRepo(*repo, cm2)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("updating the RemoteSyncer to not have any pattern")
		remotesyncer.Annotations[syngit.RtAnnotationKeyUserSpecific] = ""
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remotesyncer)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap that should not be push on any branch")
		Wait3()
		cm3 := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName3, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm3,
				metav1.CreateOptions{},
			)
			return err != nil && utils.ErrorTypeChecker(&utils.RemoteTargetNotFoundError{}, err.Error())
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is not present on any branch")
		Wait3()
		repo = &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch1,
		}
		exists, err = IsObjectInRepo(*repo, cm3)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())
		Wait3()
		repo = &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: branch2,
		}
		exists, err = IsObjectInRepo(*repo, cm3)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())
		Wait3()
		repo = &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: string(Luffy),
		}
		exists, err = IsObjectInRepo(*repo, cm3)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())

	})

	It("push not push on the user branch when the RemoteUserBinding is not managed anymore", func() {

		By("creating the RemoteUser & RemoteUserBinding for Luffy")
		luffySecretName := string(Luffy) + "-creds"
		remoteUserLuffy := &syngit.RemoteUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remoteuser-luffy",
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
		By("creating the RemoteSyncer that target everybranches")
		remotesyncer := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remoteSyncerName2,
				Namespace: namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyUserSpecific: string(syngit.RtAnnotationValueOneUserOneBranch),
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               upstreamBranch,
				DefaultUnauthorizedUserMode: syngit.Block,
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.MultipleTarget,
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

		By("creating a test configmap that should be push on the user specific branch")
		Wait3()
		cm1 := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName4, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm1,
				metav1.CreateOptions{},
			)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is present on the user specific branch")
		Wait3()
		repo := &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: string(Luffy),
		}
		exists, err := IsObjectInRepo(*repo, cm1)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())

		By("updating the RemoteUser to remove the association pattern")
		remoteUserLuffy.Annotations[syngit.RubAnnotationKeyManaged] = "false"
		Eventually(func() bool {
			err := sClient.As(Luffy).CreateOrUpdate(remoteUserLuffy)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		By("creating a test configmap that should not be pushed on the user specific branch")
		Wait3()
		cm2 := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: cmName5, Namespace: namespace},
			Data:       map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := sClient.KAs(Luffy).CoreV1().ConfigMaps(namespace).Create(ctx,
				cm2,
				metav1.CreateOptions{},
			)
			return err != nil && utils.ErrorTypeChecker(&utils.RemoteUserSearchError{}, err.Error())
		}, timeout, interval).Should(BeTrue())

		By("checking that the configmap is not present on the user specific branch")
		Wait3()
		repo = &Repo{
			Fqdn:   gitP1Fqdn,
			Owner:  giteaBaseNs,
			Name:   repo1,
			Branch: string(Luffy),
		}
		exists, err = IsObjectInRepo(*repo, cm2)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())

	})

})
