/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package tests

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("04 RemoteSyncer with excluded fields", func() {

	const (
		annotation1Key = "test-annotation1"
		annotation2Key = "test-annotation2"
		annotation3Key = "test-annotation3"
	)

	// expectExcludedFields verifies the excluded-fields contract on
	// the committed YAML: metadata.uid and metadata.managedFields absent;
	// annotation1/2 absent while annotation3 is preserved.
	expectExcludedFields := func(fx *utils.Fixture, cm *corev1.ConfigMap) {
		GinkgoHelper()

		const (
			annotation1Key = "test-annotation1"
			annotation2Key = "test-annotation2"
			annotation3Key = "test-annotation3"
		)

		By("verifying excluded fields are absent in the committed yaml")
		Eventually(func() (bool, error) {
			return fx.Git.IsFieldDefined(fx.Repo, "main", cm, "metadata.uid")
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeFalse())

		managedFieldsExists, err := fx.Git.IsFieldDefined(fx.Repo, "main", cm, "metadata.managedFields")
		Expect(err).NotTo(HaveOccurred())
		Expect(managedFieldsExists).To(BeFalse())

		hits, err := fx.Git.SearchForObjectInRepo(fx.Repo, "main", cm)
		Expect(err).NotTo(HaveOccurred())
		Expect(hits).To(HaveLen(1))

		var parsed map[string]any
		Expect(yaml.Unmarshal(hits[0].Content, &parsed)).To(Succeed())
		metadata := parsed["metadata"].(map[string]any)
		annotations, _ := metadata["annotations"].(map[string]any)
		Expect(annotations).NotTo(BeNil())
		Expect(annotations[annotation1Key]).To(BeNil())
		Expect(annotations[annotation2Key]).To(BeNil())
		Expect(annotations[annotation3Key]).To(Equal("test"))
	}

	It("excludes the listed inline fields from the git repo", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("creating the RemoteSyncer with inline ExcludedFields")
		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test4-inline",
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: "main",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBlockAppliedMessage:  utils.DefaultDeniedMessage,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFields: []string{
					".metadata.uid",
					"metadata.managedFields",
					"metadata.annotations[" + annotation1Key + "]",
					"metadata.annotations.[" + annotation2Key + "]",
				},
				Strategy:         syngit.CommitOnly,
				TargetStrategy:   syngit.OneTarget,
				RemoteRepository: fx.RepoURL(),
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{admissionv1.Create},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"configmaps"},
						},
					}},
				},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cm4-inline", Namespace: fx.Namespace,
				Annotations: map[string]string{
					annotation1Key: "test",
					annotation2Key: "test",
					annotation3Key: "test",
				},
			},
			Data: map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err != nil && strings.Contains(err.Error(), utils.DefaultDeniedMessage)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		expectExcludedFields(fx, cm)
	})

	It("excludes fields configured via a ConfigMap reference", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("creating the RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		excludedCM := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "excluded-fields", Namespace: fx.Namespace},
			Data: map[string]string{
				"excludedFields": `["metadata.uid", "metadata.managedFields", "metadata.annotations[` + annotation1Key + `]", "metadata.annotations.[` + annotation2Key + `]"]`, // nolint:lll
			},
		}
		_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
			Create(ctx, excludedCM, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		rs := &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remotesyncer-test4-cmref",
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: "main",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBlockAppliedMessage:  utils.DefaultDeniedMessage,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.Block,
				ExcludedFieldsConfigMapRef: &corev1.ObjectReference{
					Name:      "excluded-fields",
					Namespace: fx.Namespace,
				},
				Strategy:         syngit.CommitOnly,
				TargetStrategy:   syngit.OneTarget,
				RemoteRepository: fx.RepoURL(),
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{admissionv1.Create},
						Rule: admissionv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"configmaps"},
						},
					}},
				},
			},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cm4-cmref", Namespace: fx.Namespace,
				Annotations: map[string]string{
					annotation1Key: "test",
					annotation2Key: "test",
					annotation3Key: "test",
				},
			},
			Data: map[string]string{"test": "oui"},
		}
		Eventually(func() bool {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err != nil && strings.Contains(err.Error(), utils.DefaultDeniedMessage)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())

		expectExcludedFields(fx, cm)
	})
})
