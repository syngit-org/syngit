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

	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("30 Push to an existing resource", func() {

	It("updates an existing YAML file rather than creating a new one at the default path", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildDefaultCmRemoteSyncer("remotesyncer-test30-1", fx.Namespace, "main", fx.RepoURL()))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test30-1")

		cm := &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm30-1", Namespace: fx.Namespace},
			Data:       map[string]string{"test": "oui"},
		}

		By("pre-commit the ConfigMap at a custom path in the repo")
		Expect(fx.Git.CommitObject(fx.Repo, "main", "custom-path/custom.yaml", cm, "seed custom.yaml")).To(Succeed())

		By("mutating the ConfigMap and creating it on the cluster - controller should update the existing file")
		cm.Data["another"] = "test"
		Eventually(func() error {
			_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
				Create(ctx, cm, metav1.CreateOptions{})
			return err
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		By("the ConfigMap should appear at the pre-existing path (not elsewhere)")
		Eventually(func() (string, error) {
			hits, err := fx.Git.SearchForObjectInRepo(fx.Repo, "main", cm)
			if err != nil || len(hits) != 1 {
				return "", err
			}
			return hits[0].Path, nil
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Equal("custom-path/custom.yaml"))
	})

	It("updates the right document of a multi-document YAML file", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			BuildDefaultCmRemoteSyncer("remotesyncer-test30-2", fx.Namespace, "main", fx.RepoURL()))).To(Succeed())
		fx.WaitForDynamicWebhook("remotesyncer-test30-2")

		By("pre-commit a multi-document YAML file at a custom path")
		multiDoc := []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: mypod
  namespace: ` + fx.Namespace + `
---
data:
  test: oui
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm30-2
  namespace: ` + fx.Namespace + `
`)
		Expect(fx.Git.CommitFile(fx.Repo, "main", "custom-path/custom.yaml", multiDoc, "seed multi-doc")).To(Succeed())

		By("creating the ConfigMap on the cluster with different data")
		cm := CreateConfigMap(ctx, fx, "test-cm30-2", map[string]string{"test": "non"})

		By("the ConfigMap should still live inside the pre-existing file at the custom path")
		Eventually(func() (string, error) {
			hits, err := fx.Git.SearchForObjectInRepo(fx.Repo, "main", cm)
			if err != nil || len(hits) != 1 {
				return "", err
			}
			return hits[0].Path, nil
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Equal("custom-path/custom.yaml"))
	})
})
