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
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	controllerssyngit "github.com/syngit-org/syngit/internal/controller"
	webhooksyngitv1beta3 "github.com/syngit-org/syngit/internal/webhook/v1beta3"
	"github.com/syngit-org/syngit/test/utils"
	. "github.com/syngit-org/syngit/test/utils"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	syngitv1beta2 "github.com/syngit-org/syngit/pkg/api/v1beta2"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
)

const (
	timeout  = time.Second * 60
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

const (
	operatorNamespace           = "syngit"
	namespace                   = "test"
	defaultDeniedMessage        = "DENIED ON PURPOSE"
	rsPermissionsDeniedMessage  = "is not allowed to scope"
	ruPermissionsDeniedMessage  = "is not allowed to get the secret"
	rubPermissionsDeniedMessage = "is not allowed to get the referenced remoteuser"
	x509ErrorMessage            = "x509: certificate signed by unknown authority"
	crossRubErrorMessage        = "the RemoteUser is already bound in the RemoteUserBinding"
	rubNotFound                 = "no RemoteUserBinding found for the user"
	defaultUserNotFound         = "the default RemoteUser is not found"
	defaultTargetNotFound       = "the default RemoteTarget is not found"
	notPresentOnCluser          = "not found"
	sameBranchRepo              = "should not be set when the target repo & target branch are the same as the upstream repo & branch"
	rtNotFound                  = "no RemoteTarget found"
	oneTargetForMultipleMessage = "multiple RemoteTargets found for OneTarget set as the TargetStrategy in the RemoteSyncer"
)

// CMD & CLIENT
var cmd *exec.Cmd
var sClient *SyngitTestUsersClientset

// PLATFORMS FQDN
var gitP1Fqdn string
var gitP2Fqdn string

// KIND CLUSTER
var clusterAlreadyExistsBefore = false

// RBAC
const reducedPermissionsCRName = "secret-rs-ru-cluster-role"

// Dynamic webhook name
const dynamicWebhookName = "syngit-dynamic-remotesyncer-webhook"

// MANAGER
var k8sManager ctrl.Manager
var cfg *rest.Config
var testEnv *envtest.Environment

// FULL OR FAST
var setupType string

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
	RunSpecs(t, "behavior suite syngit")
}

// setupCluster creates a kind cluster if it doesn't exist using the .env file for the name.
func setupCluster() {
	By("creating the cluster")
	cmd = exec.Command("kind", "create", "cluster", "--name", os.Getenv("CLUSTER_NAME"))
	_, err := Run(cmd)
	if err != nil {
		clusterAlreadyExistsBefore = true
	}
}

// setupGitea installs the 2 gitea platforms charts and initialize the repos & users permissions.
func setupGitea() {
	By("setuping gitea repos & users")
	cmd = exec.Command("make", "setup-gitea")
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

// setupManager creates the manager and the webhooks for the tests.
func setupManager() {

	os.Setenv("MANAGER_NAMESPACE", operatorNamespace)
	os.Setenv("DYNAMIC_WEBHOOK_NAME", dynamicWebhookName)
	os.Setenv("GITEA_TEMP_CERT_DIR", "/tmp/gitea-certs")

	By("adding syngit to scheme")
	// Add the previous apiVersion for conversion
	err := syngitv1beta2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	// Add the current apiVersion
	err = syngit.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			IgnoreSchemeConvertible: true,
			Paths:                   []string{filepath.Join(".", "config", "webhook", "manifests.yaml")},
		},
		CRDDirectoryPaths: []string{filepath.Join(".", "config", "crd", "bases")},

		BinaryAssetsDirectory: filepath.Join(".", "bin", "k8s",
			fmt.Sprintf("1.29.0-%s-%s", runtime.GOOS, runtime.GOARCH)),

		CRDInstallOptions: envtest.CRDInstallOptions{
			Scheme: scheme.Scheme,
		},

		ControlPlaneStartTimeout: 5 * 30 * time.Second,
	}

	var errTest error
	cfg, errTest = testEnv.Start()
	Expect(errTest).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	errScheme := syngit.AddToScheme(scheme.Scheme)
	Expect(errScheme).NotTo(HaveOccurred())

	By("creating the manager")
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	var errK8sManager error
	k8sManager, errK8sManager = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
		}),
		LeaderElection: false,
	})
	Expect(errK8sManager).ToNot(HaveOccurred())

	By("setting up the webhooks dev variables")
	os.Setenv("DEV_MODE", "true")
	os.Setenv("DEV_WEBHOOK_HOST", webhookInstallOptions.LocalServingHost)
	os.Setenv("DEV_WEBHOOK_PORT", fmt.Sprint(webhookInstallOptions.LocalServingPort))
	os.Setenv("DEV_WEBHOOK_CERT", webhookInstallOptions.LocalServingCertDir+"/tls.crt")

	By("registring webhook server")
	errWebhook := webhooksyngitv1beta3.SetupRemoteUserWebhookWithManager(k8sManager)
	Expect(errWebhook).NotTo(HaveOccurred())
	errWebhook = webhooksyngitv1beta3.SetupRemoteSyncerWebhookWithManager(k8sManager)
	Expect(errWebhook).NotTo(HaveOccurred())
	errWebhook = webhooksyngitv1beta3.SetupRemoteUserBindingWebhookWithManager(k8sManager)
	Expect(errWebhook).NotTo(HaveOccurred())
	errWebhook = webhooksyngitv1beta3.SetupRemoteTargetWebhookWithManager(k8sManager)
	Expect(errWebhook).NotTo(HaveOccurred())
	k8sManager.GetWebhookServer().Register("/syngit-v1beta3-remoteuser-association", &webhook.Admission{Handler: &webhooksyngitv1beta3.RemoteUserAssociationWebhookHandler{
		Client:  k8sManager.GetClient(),
		Decoder: admission.NewDecoder(k8sManager.GetScheme()),
	}})
	k8sManager.GetWebhookServer().Register("/syngit-v1beta3-remoteuser-permissions", &webhook.Admission{Handler: &webhooksyngitv1beta3.RemoteUserPermissionsWebhookHandler{
		Client:  k8sManager.GetClient(),
		Decoder: admission.NewDecoder(k8sManager.GetScheme()),
	}})
	k8sManager.GetWebhookServer().Register("/syngit-v1beta3-remoteuserbinding-permissions", &webhook.Admission{Handler: &webhooksyngitv1beta3.RemoteUserBindingPermissionsWebhookHandler{
		Client:  k8sManager.GetClient(),
		Decoder: admission.NewDecoder(k8sManager.GetScheme()),
	}})
	k8sManager.GetWebhookServer().Register("/syngit-v1beta3-remotesyncer-rules-permissions", &webhook.Admission{Handler: &webhooksyngitv1beta3.RemoteSyncerWebhookHandler{
		Client:  k8sManager.GetClient(),
		Decoder: admission.NewDecoder(k8sManager.GetScheme()),
	}})
	k8sManager.GetWebhookServer().Register("/syngit-v1beta3-remotesyncer-target-pattern", &webhook.Admission{Handler: &webhooksyngitv1beta3.RemoteSyncerTargetPatternWebhookHandler{
		Client:  k8sManager.GetClient(),
		Decoder: admission.NewDecoder(k8sManager.GetScheme()),
	}})

	By("setting up the controllers")
	errController := (&controllerssyngit.RemoteUserReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(errController).ToNot(HaveOccurred())
	errController = (&controllerssyngit.RemoteUserBindingReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(errController).ToNot(HaveOccurred())
	errController = (&controllerssyngit.RemoteSyncerReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(errController).ToNot(HaveOccurred())
	errController = (&controllerssyngit.RemoteTargetReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(errController).ToNot(HaveOccurred())

	By("starting the manager")
	go func() {
		defer GinkgoRecover()
		err := k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// wait for the webhook server to get ready.
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}

		return conn.Close()
	}).Should(Succeed())
}

// rbacSetup creates the RBAC permissions of the k8s users (listed in the mock-users.go Users array).
func rbacSetup(ctx context.Context) {
	By("setting the default client successfully")
	sClient = &SyngitTestUsersClientset{}
	err := sClient.Initialize(cfg)
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
	By("creating users with reduced RBAC for Brook")
	_, err = sClient.KAs(Admin).RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: reducedPermissionsCRName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create"},
				APIGroups: []string{"", "syngit.io"},
				Resources: []string{"secrets", "remoteusers", "remoteuserbindings"},
			},
			{
				Verbs:         []string{"get", "list", "watch"},
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{string(Brook) + "-creds"},
			},
			{
				Verbs:     []string{"create", "get", "list", "watch"},
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
			},
			{
				Verbs:     []string{"create", "get", "list", "watch", "update", "delete"},
				APIGroups: []string{"syngit.io"},
				Resources: []string{"remotesyncers"},
			},
			{
				Verbs:         []string{"get", "list", "watch", "update", "delete"},
				APIGroups:     []string{"syngit.io"},
				Resources:     []string{"remoteusers"},
				ResourceNames: []string{"remoteuser-brook"},
			},
			{
				Verbs:         []string{"get", "list", "watch", "update", "delete"},
				APIGroups:     []string{"syngit.io"},
				Resources:     []string{"remoteuserbindings"},
				ResourceNames: []string{"remoteuserbinding-brook"},
			},
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	By("creating ClusterRoleBinding for the user Brook")
	_, err = sClient.KAs(Admin).RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-cluster-role-binding", string(Brook)),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User", // Represents a real user
				Name:     string(Brook),
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

	By("validating RBAC creation for the user Brook")
	crbName := fmt.Sprintf("%s-cluster-role-binding", string(Brook))
	crb, err := sClient.KAs(Admin).RbacV1().ClusterRoleBindings().Get(ctx, crbName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	Expect(crb.Subjects).To(ContainElement(rbacv1.Subject{
		Kind:     "User",
		Name:     string(Brook),
		APIGroup: "rbac.authorization.k8s.io",
	}))
}

// namespaceSetup creates the test namespace and the secrets for the users to connect to the gitea platforms.
func namespaceSetup(ctx context.Context) {

	By("creating the syngit namespace")
	_, err := sClient.KAs(Admin).CoreV1().Namespaces().Create(ctx,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: operatorNamespace}},
		metav1.CreateOptions{},
	)
	Expect(err).NotTo(HaveOccurred())

	By("creating the test namespace")
	_, err = sClient.KAs(Admin).CoreV1().Namespaces().Create(ctx,
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

// isGitlabInstalled checks if the gitea charts are installed on the 2 platform's namespace.
func isGiteaInstalled() bool {
	By("checking the gitea installation")
	cmd = exec.Command("helm", "status", "gitea", "-n", os.Getenv("PLATFORM1"))
	_, err := Run(cmd)
	if err != nil {
		return false
	}
	cmd = exec.Command("helm", "status", "gitea", "-n", os.Getenv("PLATFORM2"))
	_, err = Run(cmd)
	return err == nil
}

var _ = BeforeSuite(func() {
	ctx := context.TODO()
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	if setupType == "full" {
		setupCluster()
		setupGitea()
	}
	if setupType == "fast" && !isGiteaInstalled() {
		setupGitea()
	}

	setupManager()
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

// uninstallSetup deletes the kind cluster it did not exist before and uninstall the gitea charts.
func uninstallSetup() {
	if !clusterAlreadyExistsBefore {
		By("deleting the old cluster")
		cmd = exec.Command("kind", "delete", "cluster", "--name", os.Getenv("CLUSTER_NAME"))
		_, err := Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	By("uninstalling gitea")
	cmd = exec.Command("make", "cleanup-gitea")
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

}

// deleteRbac deletes the RBAC permissions of the k8s users.
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

	deleteRbac(ctx)

	By("tearing down the test environment")
	Eventually(func() bool {
		errTestEnv := testEnv.Stop()
		return errTestEnv == nil
	}, timeout, interval).Should(BeTrue())

	if setupType == "full" {
		uninstallSetup()
	}

})

var _ = AfterEach(func() {
	ctx := context.TODO()

	By(fmt.Sprintf("deleting the remotetargets from the %s ns", namespace))
	remoteTargets := &syngit.RemoteTargetList{}
	err := sClient.As(Admin).List(namespace, remoteTargets)
	if err == nil {
		for _, remotetarget := range remoteTargets.Items {
			nnRub := types.NamespacedName{
				Name:      remotetarget.Name,
				Namespace: remotetarget.Namespace,
			}
			rub := &syngit.RemoteTarget{}
			err = sClient.As(Admin).Get(nnRub, rub)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				err := sClient.As(Admin).Delete(rub)
				return err == nil
			}, timeout, interval).Should(BeTrue())
		}
	}

	By(fmt.Sprintf("deleting the remotesyncers from the %s ns", namespace))
	remoteSyncers := &syngit.RemoteSyncerList{}
	err = sClient.As(Admin).List(namespace, remoteSyncers)
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

// Wait3 sleeps for 3 seconds
func Wait3() {
	time.Sleep(3 * time.Second)
}
