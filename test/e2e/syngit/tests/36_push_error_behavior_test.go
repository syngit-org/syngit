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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("36 Push error behavior", func() {

	// pushFailureMessage is the substring the interceptor bubbles up to the
	// apiserver when the git push is rejected.
	const pushFailureMessage = "failed to push changes"

	// buildRS returns a RemoteSyncer on ConfigMaps whose reaction to a failing
	// push is driven by strategy and behavior. Retries are left at 0 on
	// purpose: counting them is 35's job, this spec only observes the outcome.
	buildRS := func(
		fx *utils.Fixture,
		name string,
		strategy syngit.Strategy,
		behavior syngit.PushErrorBehavior,
	) *syngit.RemoteSyncer {
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
				DefaultPushErrorBehavior:    behavior,
				Strategy:                    strategy,
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

	newCM := func(fx *utils.Fixture, name string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}
	}

	createCM := func(ctx context.Context, fx *utils.Fixture, name string) error {
		_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
			Create(ctx, newCM(fx, name), metav1.CreateOptions{})
		return err
	}

	// warmUp creates throwaway ConfigMaps until one of them makes the
	// interceptor actually attempt a push, i.e. until the RemoteUserBinding,
	// the RemoteTarget and the dynamic webhook are all live. Only after that
	// is a single create a meaningful measurement of the push error behavior.
	//
	// Unlike 35, "the create got denied" cannot be used as the readiness
	// signal here: under Pass a create always succeeds, so the spec would go
	// green even if interception never ran at all.
	warmUp := func(ctx context.Context, fx *utils.Fixture, prefix string) {
		GinkgoHelper()
		i := 0
		Eventually(func() bool {
			fx.Git.ResetPushAttempts()
			_ = createCM(ctx, fx, fmt.Sprintf("%s-warmup-%d", prefix, i))
			i++
			return fx.Git.PushAttempts(fx.Repo) > 0
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue(),
			"the interceptor never attempted a push, the pipeline was never reached")
	}

	// setup creates the managed RemoteUser, revokes Developer's push access on
	// the repo so every push is rejected, then creates the RemoteSyncer and
	// waits until the pipeline is warm.
	setup := func(ctx context.Context, fx *utils.Fixture, rs *syngit.RemoteSyncer) {
		GinkgoHelper()

		By("creating the managed RemoteUser for Developer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())
		fx.WaitForRemoteUserReady("remoteuser-developer")

		By("revoking Developer's push access on the repo")
		fx.DenyPush(utils.Developer)

		By("creating the RemoteSyncer")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("creating ConfigMaps until the interceptor attempts a push")
		warmUp(ctx, fx, rs.Name)
	}

	It("applies the object on the cluster but not on git when the behavior is Pass", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		rs := buildRS(fx, "remotesyncer-test36-1", syngit.CommitApply, syngit.Pass)
		setup(ctx, fx, rs)

		By("creating a ConfigMap that the failing push must not block")
		fx.Git.ResetPushAttempts()
		Expect(createCM(ctx, fx, "test-cm36-1")).To(Succeed())
		Expect(fx.Git.PushAttempts(fx.Repo)).To(BeNumerically(">", 0),
			"the push was never attempted, so nothing proves the create went through the pipeline")

		By("the ConfigMap is on the cluster but not on the branch")
		ExpectOnCluster(ctx, fx, "test-cm36-1")
		ExpectNotOnBranch(fx, "main", newCM(fx, "test-cm36-1"))

		By("the RemoteSyncer still reports the push failure")
		Eventually(func() bool {
			got := &syngit.RemoteSyncer{}
			err := fx.Users.CtrlAs(utils.Developer).Get(ctx,
				types.NamespacedName{Name: rs.Name, Namespace: fx.Namespace}, got)
			if err != nil {
				return false
			}
			for _, cond := range got.Status.Conditions {
				if cond.Type == "Synced" &&
					cond.Status == metav1.ConditionFalse &&
					cond.Reason == "WebhookHandlerError" {
					return true
				}
			}
			return false
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue(),
			"the RemoteSyncer never reported Synced=False/WebhookHandlerError")
	})

	It("blocks the object when the behavior is explicitly Block", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		setup(ctx, fx, buildRS(fx, "remotesyncer-test36-2", syngit.CommitApply, syngit.BlockPushError))

		By("creating a ConfigMap that the failing push must block")
		err := createCM(ctx, fx, "test-cm36-2")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(pushFailureMessage))

		By("the ConfigMap is nowhere: not in git, not on the cluster")
		ExpectNotOnBranch(fx, "main", newCM(fx, "test-cm36-2"))
		ExpectNotOnCluster(ctx, fx, "test-cm36-2")
	})

	It("blocks the object in CommitOnly mode even when the behavior is Pass", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		setup(ctx, fx, buildRS(fx, "remotesyncer-test36-3", syngit.CommitOnly, syngit.Pass))

		By("creating a ConfigMap that CommitOnly must block despite Pass")
		err := createCM(ctx, fx, "test-cm36-3")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(pushFailureMessage))

		By("the ConfigMap is nowhere: not in git, not on the cluster")
		ExpectNotOnBranch(fx, "main", newCM(fx, "test-cm36-3"))
		ExpectNotOnCluster(ctx, fx, "test-cm36-3")
	})
})
