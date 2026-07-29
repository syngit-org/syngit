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

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("35 Push error retry", func() {

	// pushFailureMessage is the substring the interceptor bubbles up to the
	// apiserver when the git push is rejected.
	const pushFailureMessage = "failed to push changes"

	// buildRS returns a CommitApply RemoteSyncer on ConfigMaps that retries a
	// failing push retries times before giving up.
	buildRS := func(fx *utils.Fixture, name string, retries int) *syngit.RemoteSyncer {
		return &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: "main",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.BlockDefaultUser,
				ExcludedFields:              []string{".metadata.uid"},
				PushErrorRetryNumber:        retries,
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            fx.RepoURL(),
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
	}

	createCM := func(ctx context.Context, fx *utils.Fixture, name string) error {
		cm := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
		_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
			Create(ctx, cm, metav1.CreateOptions{})
		return err
	}

	// setup creates the managed RemoteUser, revokes Developer's push access on
	// the repo so every push is rejected, then creates the RemoteSyncer and
	// waits until an intercepted create is actually denied because of the push
	// failure. Returns once the pipeline is warm, so the next create can be
	// measured on its own.
	setup := func(ctx context.Context, fx *utils.Fixture, rsName string, retries int) {
		GinkgoHelper()

		By("creating the managed RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		By("revoking Developer's push access on the repo")
		fx.DenyPush(utils.Developer)

		By("creating the RemoteSyncer")
		rs := buildRS(fx, rsName, retries)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("creating a ConfigMap until the push failure denies it")
		Eventually(func() bool {
			err := createCM(ctx, fx, rsName+"-warmup")
			return err != nil && strings.Contains(err.Error(), pushFailureMessage)
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	}

	It("retries the failing push .spec.pushErrorRetryNumber times", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		setup(ctx, fx, "remotesyncer-test35-1", 2)

		By("creating a ConfigMap and counting the push attempts it triggers")
		fx.Git.ResetPushAttempts()
		err := createCM(ctx, fx, "test-cm35-1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(pushFailureMessage))
		Expect(fx.Git.PushAttempts(fx.Repo)).To(Equal(3), "the initial push + 2 retries")

		By("the ConfigMap is nowhere: not in git, not on the cluster")
		ExpectNotOnBranch(fx, "main", &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm35-1", Namespace: fx.Namespace},
		})
		ExpectNotOnCluster(ctx, fx, "test-cm35-1")
	})

	It("does not retry the failing push when .spec.pushErrorRetryNumber is unset", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		setup(ctx, fx, "remotesyncer-test35-2", 0)

		By("creating a ConfigMap and counting the push attempts it triggers")
		fx.Git.ResetPushAttempts()
		err := createCM(ctx, fx, "test-cm35-2")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(pushFailureMessage))
		Expect(fx.Git.PushAttempts(fx.Repo)).To(Equal(1), "the initial push only")
	})
})
