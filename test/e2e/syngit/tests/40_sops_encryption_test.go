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

	"filippo.io/age"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sopsprovider "github.com/syngit-org/syngit-provider-sops/pkg"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	. "github.com/syngit-org/syngit/test/e2e/syngit/helpers"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("40 SOPS encryption", func() {

	It("commits the intercepted resource as a SOPS document instead of cleartext", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const (
			cmName     = "test-cm40-1"
			secretName = "sops-age40"
			secretSay  = "top-secret-value"
		)

		By("generating a throwaway age key pair")
		identity, err := age.GenerateX25519Identity()
		Expect(err).NotTo(HaveOccurred())

		By("committing a .sops.yaml at the root of the repository")
		// encrypted_regex leaves apiVersion, kind and metadata in cleartext, so
		// syngit can still locate the document on a later push.
		sopsYAML := []byte(fmt.Sprintf(`creation_rules:
  - path_regex: .*
    encrypted_regex: '^(data)$'
    age: %s
`, identity.Recipient().String()))
		Expect(fx.Git.CommitFile(fx.Repo, "main", ".sops.yaml", sopsYAML, "seed .sops.yaml")).To(Succeed())

		By("creating the Secret holding the age private key")
		ageSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: fx.Namespace},
			Data:       map[string][]byte{"age" + sopsprovider.AgeKeySecretSuffix: []byte(identity.String())},
		}
		Expect(fx.Users.CtrlAs(utils.Developer).Create(ctx, ageSecret)).To(Succeed())

		By("creating the RemoteUser & the RemoteSyncer with SOPS enabled")
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		rs := BuildDefaultCmRemoteSyncer("remotesyncer-test40-1", fx.Namespace, "main", fx.RepoURL())
		rs.Spec.SOPS = syngit.SOPSConfig{
			Enabled:   true,
			SecretRef: corev1.SecretReference{Name: secretName},
		}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("creating the ConfigMap on the cluster")
		CreateConfigMap(ctx, fx, cmName, map[string]string{"say": secretSay})

		By("the committed file must be a SOPS document with the value encrypted")
		expectedPath := fmt.Sprintf("%s/v1/configmaps/%s.yaml", fx.Namespace, cmName)
		var committed []byte
		Eventually(func(g Gomega) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", expectedPath)
			g.Expect(err).NotTo(HaveOccurred(), "expected a committed file at %q on main", expectedPath)
			g.Expect(sopsprovider.IsSopsEncrypted(content)).To(BeTrue(),
				"expected a SOPS document, got:\n%s", content)
			committed = content
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())

		Expect(string(committed)).NotTo(ContainSubstring(secretSay),
			"the value reached the repository in cleartext")

		By("the object identity must stay in cleartext so syngit can find it again")
		Expect(string(committed)).To(ContainSubstring("kind: ConfigMap"))
		Expect(string(committed)).To(ContainSubstring("name: " + cmName))

		By("the committed document must decrypt back to the original value")
		identities, err := sopsprovider.AgeIdentitiesFromSecret(ageSecret)
		Expect(err).NotTo(HaveOccurred())
		rules, err := sopsprovider.LoadCreationRule(sopsYAML, expectedPath)
		Expect(err).NotTo(HaveOccurred())
		decrypted, err := sopsprovider.Decrypt(committed,
			sopsprovider.Config{Rules: rules, Identities: identities})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(decrypted)).To(ContainSubstring(secretSay))
	})

	It("pushes in cleartext when the path matches no creation rule", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		const cmName = "test-cm40-2"

		identity, err := age.GenerateX25519Identity()
		Expect(err).NotTo(HaveOccurred())

		By("committing a .sops.yaml whose creation rule matches nothing syngit writes")
		sopsYAML := []byte(fmt.Sprintf(`creation_rules:
  - path_regex: ^vault/.*
    encrypted_regex: '^(data)$'
    age: %s
`, identity.Recipient().String()))
		Expect(fx.Git.CommitFile(fx.Repo, "main", ".sops.yaml", sopsYAML, "seed .sops.yaml")).To(Succeed())

		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer,
			fx.NewRemoteUser(utils.Developer, "remoteuser-developer", true))).To(Succeed())

		rs := BuildDefaultCmRemoteSyncer("remotesyncer-test40-2", fx.Namespace, "main", fx.RepoURL())
		rs.Spec.SOPS = syngit.SOPSConfig{Enabled: true}
		Expect(fx.Users.CreateOrUpdate(ctx, utils.Developer, rs)).To(Succeed())
		fx.WaitForDynamicWebhook(rs.Name)

		By("creating the ConfigMap on the cluster")
		CreateConfigMap(ctx, fx, cmName, map[string]string{"say": "not-a-secret"})

		By("the file is committed untouched: a non-matching path is out of the encryption scope")
		expectedPath := fmt.Sprintf("%s/v1/configmaps/%s.yaml", fx.Namespace, cmName)
		Eventually(func(g Gomega) {
			content, err := fx.Git.ReadFile(fx.Repo, "main", expectedPath)
			g.Expect(err).NotTo(HaveOccurred(), "expected a committed file at %q on main", expectedPath)
			g.Expect(sopsprovider.IsSopsEncrypted(content)).To(BeFalse(),
				"expected a cleartext document, got:\n%s", content)
			g.Expect(string(content)).To(ContainSubstring("not-a-secret"))
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	})
})
