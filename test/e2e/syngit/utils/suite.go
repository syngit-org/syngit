package utils

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2" // nolint:staticcheck
	. "github.com/onsi/gomega"    // nolint:staticcheck

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlenvtest "sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	controllerssyngit "github.com/syngit-org/syngit/internal/controller"
	webhooksyngitv1beta4 "github.com/syngit-org/syngit/internal/webhook/v1beta4"
	syngitv1beta3 "github.com/syngit-org/syngit/pkg/api/v1beta3"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	syngitenvtest "github.com/syngit-org/syngit/pkg/envtest"
	features "github.com/syngit-org/syngit/pkg/feature"
)

// Suite owns the shared state exercised by every spec: the envtest
// control plane, the controller-runtime manager, two GitServer instances
// with distinct FQDNs, and the impersonation-aware user client.
//
// A Suite is intended to be constructed once in BeforeSuite and reused
// across every spec in the package.
type Suite struct {
	TestEnv      *ctrlenvtest.Environment
	RestConfig   *rest.Config
	Manager      ctrl.Manager
	GitServer    *syngitenvtest.GitServer // primary git server
	GitServerAlt *syngitenvtest.GitServer // alternate git server, distinct FQDN
	Users        *UserClient
}

// Bootstrap starts envtest, builds the manager with every webhook and
// controller wired up, starts both GitServers, registers the 3 canonical
// users on each, and sets up cluster-wide RBAC. Returns a ready Suite.
func Bootstrap() *Suite {
	GinkgoHelper()
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	s := &Suite{}
	s.startEnvtest()
	s.startManager()

	By("starting primary git server")
	var err error
	s.GitServer, err = syngitenvtest.NewGitServer()
	Expect(err).NotTo(HaveOccurred())

	By("starting alternate git server")
	s.GitServerAlt, err = syngitenvtest.NewGitServer()
	Expect(err).NotTo(HaveOccurred())

	By("registering users on both git servers")
	for _, u := range AllUsers {
		gu := syngitenvtest.GitUser{
			Username: string(u),
			Password: DefaultPassword(u),
			Email:    DefaultEmail(u),
		}
		s.GitServer.AddUser(gu)
		s.GitServerAlt.AddUser(gu)
	}

	s.Users = NewUserClient(s.RestConfig)

	ctx := context.Background()
	By("creating operator namespace " + OperatorNamespace)
	Expect(s.createOperatorNamespace(ctx)).To(Succeed())

	By("creating cluster-wide RBAC")
	Expect(s.createClusterRBAC(ctx)).To(Succeed())

	return s
}

// Teardown stops the git servers and the envtest environment. It is
// idempotent.
func (s *Suite) Teardown() {
	if s == nil {
		return
	}
	if s.GitServer != nil {
		s.GitServer.Stop()
	}
	if s.GitServerAlt != nil {
		s.GitServerAlt.Stop()
	}
	if s.TestEnv != nil {
		Eventually(func() error { return s.TestEnv.Stop() }).WithTimeout(60 * time.Second).Should(Succeed())
	}
}

// resolveBinaryAssetsDir returns the directory that envtest should use for
// the kube-apiserver / etcd binaries. It honors the standard
// KUBEBUILDER_ASSETS env var so that Makefile targets (which call
// setup-envtest to populate it) take precedence. When that variable is
// empty (e.g. when running from the VSCode "run test" gutter button),
// fall back to bin/k8s/<version>-<os>-<arch>; the version comes from
// ENVTEST_K8S_VERSION or defaults to 1.35.0 to match the repo's Makefile.
func resolveBinaryAssetsDir(projectRoot string) string {
	if os.Getenv("KUBEBUILDER_ASSETS") != "" {
		return ""
	}
	version := os.Getenv("ENVTEST_K8S_VERSION")
	if version == "" {
		version = "1.35.0"
	}
	return filepath.Join(projectRoot, "bin", "k8s",
		fmt.Sprintf("%s-%s-%s", version, runtime.GOOS, runtime.GOARCH))
}

// ProjectRoot walks up from the working directory to find the module
// root (the directory containing go.mod / config/crd).
func ProjectRoot() string {
	wd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	dir := wd
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	Fail("could not locate project root from " + wd)
	return ""
}

// startEnvtest boots the control plane and installs CRDs + webhook manifests.
func (s *Suite) startEnvtest() {
	By("bootstrapping envtest environment")
	Expect(syngitv1beta3.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(syngit.AddToScheme(scheme.Scheme)).To(Succeed())

	projectRoot := ProjectRoot()
	s.TestEnv = &ctrlenvtest.Environment{
		WebhookInstallOptions: ctrlenvtest.WebhookInstallOptions{
			IgnoreSchemeConvertible: true,
			Paths:                   []string{filepath.Join(projectRoot, "config", "webhook", "manifests.yaml")},
		},
		CRDDirectoryPaths:        []string{filepath.Join(projectRoot, "config", "crd", "bases")},
		BinaryAssetsDirectory:    resolveBinaryAssetsDir(projectRoot),
		CRDInstallOptions:        ctrlenvtest.CRDInstallOptions{Scheme: scheme.Scheme},
		ControlPlaneStartTimeout: 150 * time.Second,
	}

	cfg, err := s.TestEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())
	s.RestConfig = cfg
}

// startManager constructs the controller-runtime manager, registers every
// webhook and controller used by syngit, and starts it asynchronously.
func (s *Suite) startManager() {
	Expect(os.Setenv("MANAGER_NAMESPACE", OperatorNamespace)).To(Succeed())
	Expect(os.Setenv("DYNAMIC_WEBHOOK_NAME", DynamicWebhookName)).To(Succeed())
	_ = features.LoadedFeatureGates.Set(fmt.Sprintf("%s=true", features.ResourceFinder))

	By("creating the manager")
	opts := &s.TestEnv.WebhookInstallOptions
	var err error
	s.Manager, err = ctrl.NewManager(s.RestConfig, ctrl.Options{
		Scheme: scheme.Scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    opts.LocalServingHost,
			Port:    opts.LocalServingPort,
			CertDir: opts.LocalServingCertDir,
		}),
		LeaderElection: false,
	})
	Expect(err).NotTo(HaveOccurred())

	By("exposing dev webhook env to internal code")
	Expect(os.Setenv("DEV_MODE", "true")).To(Succeed())
	Expect(os.Setenv("DEV_WEBHOOK_HOST", opts.LocalServingHost)).To(Succeed())
	Expect(os.Setenv("DEV_WEBHOOK_PORT", fmt.Sprint(opts.LocalServingPort))).To(Succeed())
	Expect(os.Setenv("DEV_WEBHOOK_CERT", opts.LocalServingCertDir+"/tls.crt")).To(Succeed())

	By("registering webhook handlers")
	Expect(webhooksyngitv1beta4.SetupRemoteUserWebhookWithManager(s.Manager)).To(Succeed())
	Expect(webhooksyngitv1beta4.SetupRemoteSyncerWebhookWithManager(s.Manager)).To(Succeed())
	Expect(webhooksyngitv1beta4.SetupRemoteUserBindingWebhookWithManager(s.Manager)).To(Succeed())
	Expect(webhooksyngitv1beta4.SetupRemoteTargetWebhookWithManager(s.Manager)).To(Succeed())

	ws := s.Manager.GetWebhookServer()
	dec := admission.NewDecoder(s.Manager.GetScheme())
	ws.Register("/syngit-v1beta4-remoteuser-managed",
		&webhook.Admission{Handler: &webhooksyngitv1beta4.RemoteUserManagedWebhookHandler{
			Client: s.Manager.GetClient(), Decoder: dec,
		}})
	ws.Register("/syngit-v1beta4-remoteuser-permissions",
		&webhook.Admission{Handler: &webhooksyngitv1beta4.RemoteUserPermissionsWebhookHandler{
			Client: s.Manager.GetClient(), Decoder: dec,
		}})
	ws.Register("/syngit-v1beta4-remoteuserbinding-permissions",
		&webhook.Admission{Handler: &webhooksyngitv1beta4.RemoteUserBindingPermissionsWebhookHandler{
			Client: s.Manager.GetClient(), Decoder: dec,
		}})
	ws.Register("/syngit-v1beta4-remotesyncer-rules-permissions",
		&webhook.Admission{Handler: &webhooksyngitv1beta4.RemoteSyncerWebhookHandler{
			Client: s.Manager.GetClient(), Decoder: dec,
		}})

	By("registering controllers")
	Expect((&controllerssyngit.RemoteUserReconciler{
		Client: s.Manager.GetClient(), Scheme: s.Manager.GetScheme(),
	}).SetupWithManager(s.Manager)).To(Succeed())
	Expect((&controllerssyngit.RemoteUserBindingReconciler{
		Client: s.Manager.GetClient(), Scheme: s.Manager.GetScheme(),
	}).SetupWithManager(s.Manager)).To(Succeed())
	Expect((&controllerssyngit.RemoteSyncerReconciler{
		Client: s.Manager.GetClient(), Scheme: s.Manager.GetScheme(),
	}).SetupWithManager(s.Manager)).To(Succeed())
	Expect((&controllerssyngit.RemoteTargetReconciler{
		Client: s.Manager.GetClient(), Scheme: s.Manager.GetScheme(),
	}).SetupWithManager(s.Manager)).To(Succeed())
	Expect((&controllerssyngit.AssociationPolicyReconciler{
		Client: s.Manager.GetClient(), Scheme: s.Manager.GetScheme(),
	}).SetupWithManager(s.Manager)).To(Succeed())
	Expect((&controllerssyngit.BranchTargetPolicyReconciler{
		Client: s.Manager.GetClient(), Scheme: s.Manager.GetScheme(),
	}).SetupWithManager(s.Manager)).To(Succeed())
	Expect((&controllerssyngit.UserSpecificPolicyReconciler{
		Client: s.Manager.GetClient(), Scheme: s.Manager.GetScheme(),
	}).SetupWithManager(s.Manager)).To(Succeed())

	By("starting the manager")
	go func() {
		defer GinkgoRecover()
		Expect(s.Manager.Start(ctrl.SetupSignalHandler())).To(Succeed())
	}()

	waitForWebhookServer(opts.LocalServingHost, opts.LocalServingPort)
}

// waitForWebhookServer blocks until the manager's webhook server accepts
// a TLS connection (the manager starts asynchronously).
func waitForWebhookServer(host string, port int) {
	addr := fmt.Sprintf("%s:%d", host, port)
	dialer := &net.Dialer{Timeout: time.Second}
	Eventually(func() error {
		c, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		return c.Close()
	}).WithTimeout(60 * time.Second).Should(Succeed())
}

func (s *Suite) createOperatorNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: OperatorNamespace}}
	if err := s.Users.CtrlAs(Admin).Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// createClusterRBAC wires the canonical RBAC: developer -> cluster-admin;
// restricted -> a narrow ClusterRole equivalent to Brook's legacy role.
func (s *Suite) createClusterRBAC(ctx context.Context) error {
	c := s.Users.CtrlAs(Admin)

	devCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: DeveloperBindingName},
		Subjects: []rbacv1.Subject{{
			Kind: "User", Name: string(Developer), APIGroup: "rbac.authorization.k8s.io",
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole", Name: "cluster-admin", APIGroup: "rbac.authorization.k8s.io",
		},
	}
	if err := c.Create(ctx, devCRB); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	restrictedCR := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: RestrictedClusterRole},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create"},
				APIGroups: []string{"", "syngit.io"},
				Resources: []string{"secrets", "remoteusers", "remoteuserbindings"},
			},
			{
				Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
				APIGroups: []string{"syngit.io"},
				Resources: []string{"remotesyncers"},
			},
			{
				Verbs:         []string{"get", "list", "watch", "update", "delete"},
				APIGroups:     []string{"syngit.io"},
				Resources:     []string{"remoteusers"},
				ResourceNames: []string{"remoteuser-restricted"},
			},
			{
				Verbs:         []string{"get", "list", "watch", "update", "delete"},
				APIGroups:     []string{"syngit.io"},
				Resources:     []string{"remoteuserbindings"},
				ResourceNames: []string{"remoteuserbinding-restricted", fmt.Sprintf("%s-%s", syngit.RubNamePrefix, SanitizeUser(Restricted))}, // nolint:lll
			},
			{
				Verbs:         []string{"get", "list", "watch"},
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{string(Restricted) + "-creds"},
			},
		},
	}
	if err := c.Create(ctx, restrictedCR); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	restrictedCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: RestrictedBindingName},
		Subjects: []rbacv1.Subject{{
			Kind: "User", Name: string(Restricted), APIGroup: "rbac.authorization.k8s.io",
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole", Name: RestrictedClusterRole, APIGroup: "rbac.authorization.k8s.io",
		},
	}
	if err := c.Create(ctx, restrictedCRB); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
