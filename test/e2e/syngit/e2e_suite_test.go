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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/syngit-org/syngit/test/utils"
	. "github.com/syngit-org/syngit/test/utils"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	syngit "github.com/syngit-org/syngit/api/v1beta2"
)

const (
	timeout  = time.Second * 60
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

const operatorNamespace = "syngit"
const namespace = "test"
const defaultDeniedMessage = "DENIED ON PURPOSE"
const permissionsDeniedMessage = "is not allowed to scope"

var cmd *exec.Cmd
var sClient *SyngitTestUsersClientset

const projectimage = "local/syngit-controller:dev"

var setupType string
var gitP1Fqdn string
var gitP2Fqdn string

func init() {
	flag.StringVar(&setupType, "setup", "full", "Specify the setup type: fast or full")
}

// Run e2e tests using the Ginkgo runner.
func TestE2E(t *testing.T) {
	projectDir, err := utils.GetProjectDir()
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "Failed to get project dir: %v\n", err)
	}
	if err := godotenv.Load(projectDir + "/test/utils/.env"); err != nil {
		fmt.Fprintf(GinkgoWriter, "Failed to load .env file: %v\n", err)
	}

	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting syngit suite\n")
	flag.Parse()
	RunSpecs(t, "e2e suite syngit")
}

const reducedPermissionsCRName = "secret-rs-ru-cluster-role"

func installationSetup() {
	By("setuping gitea repos & users")
	cmd = exec.Command("make", "setup-gitea")
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("installing prometheus operator")
	Expect(InstallPrometheusOperator()).To(Succeed())

	By("installing the cert-manager")
	Expect(InstallCertManager()).To(Succeed())

	By("loading the the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectimage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("installing the syngit chart")
	cmd = exec.Command("make", "chart-install")
	_, err = Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

func rbacSetup(ctx context.Context) {
	By("setting the default client successfully")
	sClient = &SyngitTestUsersClientset{}
	err := sClient.Initialize()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("creating users with RBAC cluster-admin for global users")
	for _, username := range append(FullPermissionsUsers, Admin) {
		By(fmt.Sprintf("creating ClusterRoleBinding for the user %s", username))
		_, err = sClient.KAs(Admin).RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-cluster-role-binding", username),
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:     "User", // Represents a real user
					Name:     string(username),
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind:     "ClusterRole",
				Name:     "cluster-admin",
				APIGroup: "rbac.authorization.k8s.io",
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("validating RBAC creation for the user %s", username))
		crbName := fmt.Sprintf("%s-cluster-role-binding", username)
		crb, err := sClient.KAs(Admin).RbacV1().ClusterRoleBindings().Get(ctx, crbName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(crb.Subjects).To(ContainElement(rbacv1.Subject{
			Kind:     "User",
			Name:     string(username),
			APIGroup: "rbac.authorization.k8s.io",
		}))
	}
	By("creating users with reduced RBAC for reduced users")
	_, err = sClient.KAs(Admin).RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: reducedPermissionsCRName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create", "get", "list", "watch"},
				APIGroups: []string{""},
				Resources: []string{"namespaces", "secrets"},
			},
			{
				Verbs:     []string{"create", "get", "list", "watch", "update", "delete"},
				APIGroups: []string{"syngit.io"},
				Resources: []string{"remotesyncers", "remoteusers"},
			},
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	for _, username := range ReducedPermissionsUsers {
		By(fmt.Sprintf("creating ClusterRoleBinding for the user %s", username))
		_, err = sClient.KAs(Admin).RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-cluster-role-binding", username),
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:     "User", // Represents a real user
					Name:     string(username),
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind:     "ClusterRole",
				Name:     reducedPermissionsCRName,
				APIGroup: "rbac.authorization.k8s.io",
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("validating RBAC creation for the user %s", username))
		crbName := fmt.Sprintf("%s-cluster-role-binding", username)
		crb, err := sClient.KAs(Admin).RbacV1().ClusterRoleBindings().Get(ctx, crbName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(crb.Subjects).To(ContainElement(rbacv1.Subject{
			Kind:     "User",
			Name:     string(username),
			APIGroup: "rbac.authorization.k8s.io",
		}))
	}
}

func namespaceSetup(ctx context.Context) {

	By("creating the test namespace")
	_, err := sClient.KAs(Admin).CoreV1().Namespaces().Create(ctx,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}},
		metav1.CreateOptions{},
	)
	Expect(err).NotTo(HaveOccurred())

	for _, username := range Users {
		By(fmt.Sprintf("testing the impersonation for the user %s", username))
		namespaces, err := sClient.KAs(username).CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(namespaces.Items).NotTo(BeEmpty(), "User should be able to list namespaces")

		By(fmt.Sprintf("creating the Secret creds (to connect to jupyter & saturn) for %s", username))
		secretName := string(username) + "-creds"
		secretCreds := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			StringData: map[string]string{
				"username": string(username),
				"password": string(username) + "-pwd",
			},
			Type: "kubernetes.io/basic-auth",
		}
		_, err = sClient.KAs(username).CoreV1().Secrets(namespace).Create(ctx,
			secretCreds,
			metav1.CreateOptions{},
		)
		Expect(err).NotTo(HaveOccurred())
	}
}

func isSetupInstalled() bool {
	By("checking the gitea installation")
	cmd = exec.Command("helm", "status", "gitea", "-n", os.Getenv("PLATFORM1"))
	_, err := Run(cmd)
	if err != nil {
		return false
	}
	cmd = exec.Command("helm", "status", "gitea", "-n", os.Getenv("PLATFORM2"))
	_, err = Run(cmd)
	if err != nil {
		return false
	}

	By("checking the cert-manager installation")
	cmd = exec.Command("helm", "status", "cert-manager", "-n", "cert-manager")
	_, err = Run(cmd)
	if err != nil {
		return false
	}

	By("checking the syngit installation")
	cmd = exec.Command("helm", "status", "syngit", "-n", "syngit")
	_, err = Run(cmd)
	if err != nil { //nolint:gosimple
		return false
	}

	return true
}

var _ = BeforeSuite(func() {
	ctx := context.TODO()
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	if setupType == "full" {
		installationSetup()
	}
	if setupType == "fast" && !isSetupInstalled() {
		installationSetup()
	}
	rbacSetup(ctx)
	namespaceSetup(ctx)

	By("retrieving the gitea urls")
	var err error
	gitP1Fqdn, err = GetGiteaURL(os.Getenv("PLATFORM1"))
	Expect(err).NotTo(HaveOccurred())
	fmt.Printf("  Gitea URL for %s: %s\n", os.Getenv("PLATFORM1"), gitP1Fqdn)
	gitP2Fqdn, err = GetGiteaURL(os.Getenv("PLATFORM2"))
	Expect(err).NotTo(HaveOccurred())
	fmt.Printf("  Gitea URL for %s: %s\n", os.Getenv("PLATFORM2"), gitP2Fqdn)
})

func uninstallSetup() {
	By("uninstalling gitea")
	cmd = exec.Command("make", "cleanup-gitea")
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("uninstalling the cert-manager bundle")
	UninstallCertManager()

	By("uninstalling the syngit chart")
	cmd = exec.Command("make", "chart-uninstall")
	_, err = Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("deleting the manager namespace")
	cmd = exec.Command("kubectl", "delete", "ns", operatorNamespace)
	_, err = Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

func deleteNamespace(ctx context.Context) {

	By("deleting the test namespace")
	err := sClient.KAs(Admin).CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{GracePeriodSeconds: func() *int64 { i := int64(0); return &i }()})
	Expect(err).NotTo(HaveOccurred())
}

func deleteRbac(ctx context.Context) {

	By("deleting the global user's RBAC")
	err := sClient.KAs(Admin).RbacV1().ClusterRoles().Delete(ctx, reducedPermissionsCRName, metav1.DeleteOptions{})
	Expect(err).NotTo(HaveOccurred())
	for _, username := range append(Users, Admin) {
		By(fmt.Sprintf("deleting RBAC for the user %s", username))
		err = sClient.KAs(Admin).RbacV1().ClusterRoleBindings().Delete(ctx, fmt.Sprintf("%s-cluster-role-binding", username), metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	}

}

var _ = AfterSuite(func() {
	ctx := context.TODO()
	if setupType == "full" {
		uninstallSetup()
	}
	By("uninstalling the Prometheus manager bundle")
	UninstallPrometheusOperator()
	deleteNamespace(ctx)
	deleteRbac(ctx)
})

var _ = AfterEach(func() {
	ctx := context.TODO()

	By(fmt.Sprintf("deleting the remotesyncers from the %s ns", namespace))
	remoteSyncers := &syngit.RemoteSyncerList{}
	err := sClient.As(Admin).List(namespace, remoteSyncers)
	if err == nil {
		for _, remotesyncer := range remoteSyncers.Items {
			nnRs := types.NamespacedName{
				Name:      remotesyncer.Name,
				Namespace: remotesyncer.Namespace,
			}
			rs := &syngit.RemoteSyncer{}
			err = sClient.As(Admin).Get(nnRs, rs)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				err := sClient.As(Admin).Delete(remotesyncer.DeepCopy())
				return err == nil
			}, timeout, interval).Should(BeTrue())
		}
	}

	By(fmt.Sprintf("deleting the remoteuserbindings from the %s ns", namespace))
	remoteUserBindings := &syngit.RemoteUserBindingList{}
	err = sClient.As(Admin).List(namespace, remoteUserBindings)
	if err == nil {
		for _, remoteuserbinding := range remoteUserBindings.Items {
			nnRub := types.NamespacedName{
				Name:      remoteuserbinding.Name,
				Namespace: remoteuserbinding.Namespace,
			}
			rub := &syngit.RemoteUserBinding{}
			err = sClient.As(Admin).Get(nnRub, rub)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				err := sClient.As(Admin).Delete(rub)
				return err == nil
			}, timeout, interval).Should(BeTrue())
		}
	}

	By(fmt.Sprintf("deleting the remoteusers from the %s ns", namespace))
	remoteUsers := &syngit.RemoteUserList{}
	err = sClient.As(Admin).List(namespace, remoteUsers)
	if err == nil {
		for _, remoteuser := range remoteUsers.Items {
			nnRu := types.NamespacedName{
				Name:      remoteuser.Name,
				Namespace: remoteuser.Namespace,
			}
			ru := &syngit.RemoteUser{}
			err = sClient.As(Admin).Get(nnRu, ru)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				err := sClient.As(Admin).Delete(ru)
				return err == nil
			}, timeout, interval).Should(BeTrue())
		}
	}

	By(fmt.Sprintf("deleting the test configmaps from the %s ns", namespace))
	cms, err := sClient.KAs(Admin).CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, cm := range cms.Items {
			if strings.HasPrefix(cm.Name, "test-") {
				Eventually(func() bool {
					err = sClient.KAs(Admin).CoreV1().ConfigMaps(namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{})
					return err == nil
				}, timeout, interval).Should(BeTrue())
			}
		}
	}

	By(fmt.Sprintf("deleting the test secrets from the %s ns", namespace))
	secrets, err := sClient.KAs(Admin).CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, secret := range secrets.Items {
			if strings.HasPrefix(secret.Name, "test-") {
				Eventually(func() bool {
					err = sClient.KAs(Admin).CoreV1().Secrets(namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{})
					return err == nil
				}, timeout, interval).Should(BeTrue())
			}
		}
	}

})

func Wait5() {
	time.Sleep(5 * time.Second)
}

func Wait10() {
	time.Sleep(10 * time.Second)
}
