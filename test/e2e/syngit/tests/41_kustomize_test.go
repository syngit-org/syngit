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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kustomizeprovider "github.com/syngit-org/syngit-provider-kustomize/pkg"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("41 Kustomize provider writes into the overlay of a bundle", func() {

	const (
		overlayDir           = "apps/demo/overlays/staging"
		overlayKustomization = overlayDir + "/kustomization.yaml"
		baseConfigMap        = "apps/demo/base/configmap.yaml"
	)

	seedBundle := func(fx *utils.Fixture) {
		files := map[string]string{
			"apps/demo/base/kustomization.yaml": "resources:\n  - configmap.yaml\n",
			baseConfigMap: `apiVersion: v1
kind: ConfigMap
metadata:
  name: demo
data:
  greeting: previous
`,
			overlayKustomization: `resources:
  - ../../base
labels:
  - pairs:
      kustomize.syngit.io/bundle: demo
      kustomize.syngit.io/overlay: staging
commonAnnotations:
  kustomize.syngit.io/root: apps/demo
`,
		}
		for path, content := range files {
			Expect(fx.Git.CommitFile(fx.Repo, "main", path, []byte(content), "seed "+path)).To(Succeed())
		}
	}

	kustomizeSyncer := func(fx *utils.Fixture, name string) *syngit.RemoteSyncer {
		return &syngit.RemoteSyncer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: fx.Namespace,
				Annotations: map[string]string{
					syngit.RtAnnotationKeyOneOrManyBranches: "main",
					kustomizeprovider.ProviderAnnotation:    "enabled",
				},
			},
			Spec: syngit.RemoteSyncerSpec{
				InsecureSkipTlsVerify:       true,
				DefaultBranch:               "main",
				DefaultUnauthorizedUserMode: syngit.BlockDefaultUser,
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            fx.RepoURL(),
				ResourceFinder:              true,
				// The provider compares the live object with the git build, which
				// has none of the server-managed fields.
				ExcludedFields: []string{
					".metadata.uid",
					".metadata.creationTimestamp",
					".metadata.managedFields",
				},
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

	bundleConfigMap := func(fx *utils.Fixture, name string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: fx.Namespace,
				Labels: map[string]string{
					"kustomize.syngit.io/bundle":  "demo",
					"kustomize.syngit.io/overlay": "staging",
				},
				Annotations: map[string]string{kustomizeprovider.RootAnnotation: "apps/demo"},
			},
			Data: map[string]string{"greeting": "hello"},
		}
	}

	setup := func(ctx context.Context, fx *utils.Fixture, rsName string) {
		seedBundle(fx)
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, kustomizeSyncer(fx, rsName))).To(Succeed())
		fx.WaitForDynamicWebhook(rsName)
	}

	It("patches a base object from the overlay", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		setup(ctx, fx, "remotesyncer-test41-patch")

		By("creating the base ConfigMap with changed data")
		Eventually(func() error {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, bundleConfigMap(fx, "demo"), metav1.CreateOptions{})
			return err
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("checking the change is pushed as an overlay patch")
		Eventually(func(g Gomega) {
			patch, err := fx.Git.ReadFile(fx.Repo, "main", overlayDir+"/configmap-demo.patch.yaml")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(string(patch)).To(ContainSubstring("greeting: hello"))

			kustomization, err := fx.Git.ReadFile(fx.Repo, "main", overlayKustomization)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(string(kustomization)).To(ContainSubstring("path: configmap-demo.patch.yaml"))
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		base, err := fx.Git.ReadFile(fx.Repo, "main", baseConfigMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(base)).To(ContainSubstring("greeting: previous"))
	})

	It("denies a bundle object absent from the overlay build", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)
		setup(ctx, fx, "remotesyncer-test41-deny")

		By("creating a ConfigMap without the overlay-only annotation")
		_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
			Create(ctx, bundleConfigMap(fx, "unknown"), metav1.CreateOptions{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(kustomizeprovider.OverlayOnlyAnnotation))

		ExpectNotOnCluster(ctx, fx, "unknown")
	})
})
