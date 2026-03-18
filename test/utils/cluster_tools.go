package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck
)

const (
	prometheusOperatorVersion = "v0.68.0"
	prometheusOperatorURL     = "https://github.com/prometheus-operator/prometheus-operator/" +
		"releases/download/%s/bundle.yaml"

	certmanagerVersion = "v1.17.2"
	certmanagerCRDsURL = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.crds.yaml"

	certmanagerURLTmpl = "https://github.com/jetstack/cert-manager/releases/download/%s/cert-manager.yaml"
)

// InstallPrometheusOperator installs the prometheus Operator to be used to export the enabled metrics.
func InstallPrometheusOperator() error {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "create", "-f", url)
	_, err := Run(cmd)
	return err
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) ([]byte, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		fmt.Fprintf(GinkgoWriter, "chdir dir: %s\n", err) //nolint
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, err := fmt.Fprintf(GinkgoWriter, "running: %s\n", command)
	if err != nil {
		return nil, fmt.Errorf("%s failed with error: (%v)", command, err)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s failed with error: (%v) %s", command, err, string(output))
	}

	return output, nil
}

// UninstallPrometheusOperator uninstalls the prometheus
func UninstallPrometheusOperator() {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// UninstallCertManager uninstalls the cert manager

func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
	cmd = exec.Command("helm", "uninstall", "-n", "cert-manager", "cert-manager")
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

func UninstallCertManagerCRDs() {
	url := fmt.Sprintf(certmanagerCRDsURL, certmanagerVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

func InstallCertManagerCRDs() error {
	url := fmt.Sprintf(certmanagerCRDsURL, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url)
	_, err := Run(cmd)
	if err != nil {
		warnError(err)
	}

	return err
}

// InstallCertManager installs the full cert-manager and waits for the webhook to be ready.
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// Wait for all cert-manager deployments to be available
	for _, deploy := range []string{"cert-manager", "cert-manager-webhook", "cert-manager-cainjector"} {
		cmd = exec.Command("kubectl", "wait", "--for=condition=Available",
			fmt.Sprintf("deployment/%s", deploy), "-n", "cert-manager", "--timeout=120s")
		if _, err := Run(cmd); err != nil {
			return err
		}
	}

	// Verify the webhook is fully operational (CA bundle injected) by dry-run creating an Issuer
	for i := 0; i < 24; i++ {
		cmd = exec.Command("kubectl", "apply", "--dry-run=server", "-f", "-")
		cmd.Stdin = strings.NewReader(`apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: test-webhook-ready
  namespace: default
spec:
  selfSigned: {}
`)
		if _, err := Run(cmd); err == nil {
			return nil
		}
		fmt.Fprintf(GinkgoWriter, "cert-manager webhook not ready yet, retrying in 5s...\n") // nolint:errcheck
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("cert-manager webhook did not become ready within 120s")
}
