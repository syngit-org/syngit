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

	kustomizeprovider "github.com/syngit-org/syngit-provider-kustomize/pkg"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

var _ = Describe("41 Kustomize overlay", func() {

	const (
		bundlePath  = "apps/web"
		baseFile    = bundlePath + "/base/deployment.yaml"
		overlayFile = bundlePath + "/overlays/production/deployment.yaml"
	)

	baseDeployment := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  labels:
    tier: frontend
spec:
  replicas: 2
  selector:
    matchLabels:
      tier: frontend
  template:
    metadata:
      labels:
        tier: frontend
    spec:
      containers:
        - name: web
          image: nginx:1.25.0
`)

	// seedBundle commits the bundle of the kustomize provider's basic example: a
	// base deployment and a production overlay patching its replicas. The overlay
	// injects into the fixture namespace, which is where the intercepted object
	// lives.
	seedBundle := func(fx *utils.Fixture) {
		GinkgoHelper()
		overlayKustomization := fmt.Sprintf(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namePrefix: prod-
namespace: %s
labels:
  - pairs:
      app.kubernetes.io/part-of: myapp
    includeSelectors: true
commonAnnotations:
  managed-by: syngit
resources:
  - ../../base
patches:
  - path: deployment.yaml
`, fx.Namespace)

		Expect(fx.Git.CommitFiles(fx.Repo, "main", map[string][]byte{
			bundlePath + "/base/kustomization.yaml": []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - deployment.yaml
`),
			baseFile: baseDeployment,
			bundlePath + "/overlays/production/kustomization.yaml": []byte(overlayKustomization),
			overlayFile: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 3
`),
		}, "seed the kustomize bundle")).To(Succeed())
	}

	kustomizeSyncer := func(fx *utils.Fixture, name string) *syngit.RemoteSyncer {
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
				Strategy:                    syngit.CommitApply,
				TargetStrategy:              syngit.OneTarget,
				RemoteRepository:            fx.RepoURL(),
				ResourceFinder:              true,
				ExcludedFields: []string{
					".metadata.uid",
					".metadata.resourceVersion",
					".metadata.creationTimestamp",
					".metadata.generation",
					".metadata.managedFields",
					".status",
				},
				Kustomize: syngit.KustomizeConfig{
					Enabled:  true,
					Override: kustomizeprovider.Overlay,
				},
				ScopedResources: syngit.ScopedResources{
					Rules: []admissionv1.RuleWithOperations{{
						Operations: []admissionv1.OperationType{admissionv1.Create},
						Rule: admissionv1.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"deployments"},
						},
					}},
				},
			},
		}
	}

	// The base deployment as `kustomize build` renders it.
	// Two changes the example makes there: replicas 3 -> 5 and
	// nginx:1.25.0 -> nginx:1.27.0.
	renderedDeployment := func(fx *utils.Fixture) *appsv1.Deployment {
		labels := map[string]string{"app.kubernetes.io/part-of": "myapp", "tier": "frontend"}
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prod-web",
				Namespace: fx.Namespace,
				Labels:    labels,
				Annotations: map[string]string{
					"managed-by":                           "syngit",
					kustomizeprovider.BundlePathAnnotation: bundlePath,
					kustomizeprovider.OverlayAnnotation:    "production",
					kustomizeprovider.BaseNameAnnotation:   "web",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(5)),
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      labels,
						Annotations: map[string]string{"managed-by": "syngit"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "web", Image: "nginx:1.27.0"}},
					},
				},
			},
		}
	}

	It("commits the intercepted Deployment as the patch of its overlay", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		By("seeding the kustomize bundle")
		seedBundle(fx)

		By("creating the RemoteUser & the RemoteSyncer with Kustomize enabled")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		rs := kustomizeSyncer(fx, "remotesyncer-test41-1")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("creating the rendered Deployment on the cluster")
		Eventually(func() error {
			_, err := fx.Users.KAs(utils.Developer).AppsV1().Deployments(fx.Namespace).
				Create(ctx, renderedDeployment(fx), metav1.CreateOptions{})
			return err
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("the overlay patch must carry both changes, stripped of what the overlay injects")
		patch := &appsv1.Deployment{}
		Eventually(func(g Gomega) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", overlayFile)
			g.Expect(err).NotTo(HaveOccurred(), "expected a committed file at %q on main", overlayFile)
			g.Expect(yaml.Unmarshal(content, patch)).To(Succeed())
			g.Expect(patch.Spec.Replicas).To(HaveValue(Equal(int32(5))))
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		Expect(patch.Name).To(Equal("web"), "the namePrefix reached the repository")
		Expect(patch.Namespace).To(BeEmpty(), "the injected namespace reached the repository")
		Expect(patch.Labels).NotTo(HaveKey("app.kubernetes.io/part-of"))
		Expect(patch.Annotations).NotTo(HaveKey("managed-by"))
		Expect(patch.Spec.Template.Spec.Containers).To(HaveLen(1))
		Expect(patch.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.27.0"))

		By("the base must be left untouched")
		committedBase, err := fx.Git.ReadFile(fx.Repo, "main", baseFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(committedBase).To(Equal(baseDeployment))
	})
})
