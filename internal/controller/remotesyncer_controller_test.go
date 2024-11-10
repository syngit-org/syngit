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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	syngit "syngit.io/syngit/api/v1beta1"
)

var _ = Describe("RemoteSyncer Controller", func() {

	const (
		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250

		userNamespace = "default"
	)

	Context("When reconciling a resource", func() {

		const (
			remotesyncername = "test-remotesyncer"
		)

		typeNamespacedName := types.NamespacedName{
			Name:      remotesyncername,
			Namespace: userNamespace,
		}
		remotesyncer := &syngit.RemoteSyncer{}

		ctx := context.Background()

		BeforeEach(func() {

			By("Creating a RemoteSyncer with")
			err := k8sClient.Get(ctx, typeNamespacedName, remotesyncer)
			if err != nil && errors.IsNotFound(err) {
				resource := &syngit.RemoteSyncer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      remotesyncername,
						Namespace: userNamespace,
					},
					Spec: syngit.RemoteSyncerSpec{
						DefaultBlockAppliedMessage:  "test",
						DefaultBranch:               "main",
						DefaultUnauthorizedUserMode: syngit.Block,
						ExcludedFields:              []string{".metadata.uid"},
						ProcessMode:                 syngit.CommitOnly,
						PushMode:                    syngit.SameBranch,
						RemoteRepository:            "https://dummy-git-server.com",
						ScopedResources: syngit.ScopedResources{
							Rules: []admissionv1.RuleWithOperations{admissionv1.RuleWithOperations{
								Operations: []admissionv1.OperationType{
									admissionv1.Create,
								},
								Rule: admissionv1.Rule{
									APIGroups:   []string{"v1"},
									APIVersions: []string{"v1"},
									Resources:   []string{"configmaps"},
								},
							},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			err := k8sClient.Get(ctx, typeNamespacedName, remotesyncer)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance RemoteSyncer")
			Expect(k8sClient.Delete(ctx, remotesyncer)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			ruLookupKeyRS := types.NamespacedName{Name: remotesyncername, Namespace: userNamespace}
			createdRemoteSyncer := &syngit.RemoteSyncer{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, ruLookupKeyRS, createdRemoteSyncer)
				return err == nil
			}, timeout, interval).Should(BeTrue())
		})
	})
})
