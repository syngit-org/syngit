package helpers

import (
	"context"

	. "github.com/onsi/ginkgo/v2" // nolint:staticcheck
	. "github.com/onsi/gomega"    // nolint:staticcheck

	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// CreateConfigMap creates a ConfigMap through the k8s API as Developer and
// waits for the creation to succeed. Returns the constructed object.
func CreateConfigMap(ctx context.Context, fx *utils.Fixture, name string, data map[string]string) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fx.Namespace},
		Data:       data,
	}
	Eventually(func() error {
		_, err := fx.Users.KAs(utils.Developer).CoreV1().ConfigMaps(fx.Namespace).
			Create(ctx, cm, metav1.CreateOptions{})
		return err
	}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
	return cm
}

// ExpectOnBranch asserts the ConfigMap is present in the given branch of
// the fixture's repo.
func ExpectOnBranch(fx *utils.Fixture, branch string, cm *corev1.ConfigMap) {
	GinkgoHelper()
	Eventually(func() (bool, error) {
		return fx.Git.IsObjectInRepo(fx.Repo, branch, cm)
	}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
}

// ExpectNotOnBranch asserts the ConfigMap is absent from the given branch.
func ExpectNotOnBranch(fx *utils.Fixture, branch string, cm *corev1.ConfigMap) {
	GinkgoHelper()
	Eventually(func() (bool, error) {
		return fx.Git.IsObjectInRepo(fx.Repo, branch, cm)
	}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeFalse())
}

// ExpectOnCluster asserts the ConfigMap exists on the cluster.
func ExpectOnCluster(ctx context.Context, fx *utils.Fixture, name string) {
	GinkgoHelper()
	Eventually(func() error {
		return fx.Users.CtrlAs(utils.Developer).Get(ctx,
			types.NamespacedName{Name: name, Namespace: fx.Namespace}, &corev1.ConfigMap{})
	}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(Succeed())
}
