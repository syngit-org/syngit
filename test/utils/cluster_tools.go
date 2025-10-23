package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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

// TODO: delete following function when last stable version of syngit Helm chat is >= 0.4.8
func InstallCertManager() error {
	cmd := exec.Command("helm", "repo", "add", "jetstack", "https://charts.jetstack.io")
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}

	cmd = exec.Command("helm", "install", "cert-manager", "-n", "cert-manager", "--version", "v1.17.2", "--create-namespace", "jetstack/cert-manager", "--set", "installCRDs=true") //nolint:lll
	if _, err := Run(cmd); err != nil {
		return err
	}
	// Wait for cert-manager-webhook to be ready, which can take time if cert-manager
	// was re-installed after uninstalling on a cluster.
	cmd = exec.Command("kubectl", "wait", "deployment.apps/cert-manager-webhook",
		"--for", "condition=Available",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)

	_, err := Run(cmd)
	if err != nil {
		return err
	}

	return err
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
