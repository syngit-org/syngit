package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"k8s.io/client-go/rest"
)

func NewDefaultHelmActionConfig(chart Chart) (*action.Configuration, *cli.EnvSettings, error) {
	settings := cli.New()
	actionConfig := new(action.Configuration)
	err := actionConfig.Init(settings.RESTClientGetter(), chart.GetReleaseNamespace(), "secrets", log.Printf)
	return actionConfig, settings, err
}

func NewEnvtestHelmActionConfig(cfg *rest.Config, namespace string) (*action.Configuration, *cli.EnvSettings, error) {
	kubeconfig, err := GetKubeconfigFromConfig(cfg)
	if err != nil {
		return nil, nil, err
	}

	settings := cli.New()
	tmpKube := filepath.Join(os.TempDir(), "envtest-kubeconfig.yaml")
	if err := os.WriteFile(tmpKube, kubeconfig, 0600); err != nil {
		return nil, settings, err
	}
	settings.KubeConfig = string(kubeconfig)
	settings.RepositoryCache = "/tmp/helmcache"
	settings.RepositoryConfig = "/tmp/helmrepo"

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, "secrets", log.Panicf); err != nil {
		return nil, settings, err
	}
	return actionConfig, settings, nil
}

type Chart interface {
	GetChartPath(settings *cli.EnvSettings) (string, error)
	GetValuesPath() string
	GetChartName() string
	GetChartVersion() string
	GetReleaseName() string
	GetReleaseNamespace() string
}

type BaseChart struct {
	ValuesPath       string
	ChartName        string
	ChartVersion     string
	ReleaseName      string
	ReleaseNamespace string
}

func (chart BaseChart) GetValuesPath() string {
	return chart.ValuesPath
}
func (chart BaseChart) GetChartName() string {
	return chart.ChartName
}
func (chart BaseChart) GetChartVersion() string {
	return chart.ChartVersion
}
func (chart BaseChart) GetReleaseName() string {
	return chart.ReleaseName
}
func (chart BaseChart) GetReleaseNamespace() string {
	return chart.ReleaseNamespace
}

type LocalChart struct {
	BaseChart
	ChartPath string
}

type RemoteChart struct {
	BaseChart
	RepoURL       string
	InstallAction *action.Install
	UpgradeAction *action.Upgrade
}

func (c LocalChart) GetChartPath(settings *cli.EnvSettings) (string, error) {
	return c.ChartPath + "/" + c.ChartVersion, nil
}

func (c RemoteChart) GetChartPath(settings *cli.EnvSettings) (string, error) {
	var chartPath string
	var err error
	if c.InstallAction != nil {
		chartPath, err = c.InstallAction.LocateChart(c.ChartName, settings)
		if err != nil {
			return "", err
		}
	}
	if c.UpgradeAction != nil {
		chartPath, err = c.UpgradeAction.LocateChart(c.ChartName, settings)
		if err != nil {
			return "", err
		}
	}
	return chartPath, nil
}

func InstallChart(chart Chart, actionConfig *action.Configuration, settings *cli.EnvSettings) error {
	install := action.NewInstall(actionConfig)
	install.ReleaseName = chart.GetReleaseName()
	install.Namespace = chart.GetReleaseNamespace()
	install.CreateNamespace = true
	install.Timeout = time.Minute * 10
	install.Wait = true
	install.WaitForJobs = true

	remote, ok := chart.(RemoteChart)
	if ok {
		remote.InstallAction = install
		chart = remote
	}
	chartPath, err := chart.GetChartPath(settings)
	if err != nil {
		return err
	}

	valsOpt := &values.Options{
		ValueFiles: []string{chart.GetValuesPath()},
	}
	vals, err := valsOpt.MergeValues(getter.All(settings))
	if err != nil {
		return err
	}

	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return err
	}

	_, err = install.Run(chartRequested, vals)
	if err != nil {
		return err
	}

	return nil
}

func UninstallChart(chart Chart, actionConfig *action.Configuration, settings *cli.EnvSettings) error {
	uninstall := action.NewUninstall(actionConfig)
	uninstall.KeepHistory = false

	_, err := uninstall.Run(chart.GetReleaseName())
	if err != nil {
		return err
	}

	return nil
}

func UpgradeChart(chart Chart, actionConfig *action.Configuration, settings *cli.EnvSettings) error {
	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Namespace = chart.GetReleaseNamespace()
	upgrade.Timeout = time.Minute * 10
	upgrade.Wait = true
	upgrade.WaitForJobs = true

	remote, ok := chart.(RemoteChart)
	if ok {
		remote.UpgradeAction = upgrade
		chart = remote
	}
	chartPath, err := chart.GetChartPath(settings)
	if err != nil {
		return err
	}

	valsOpt := &values.Options{
		ValueFiles: []string{chart.GetValuesPath()},
	}
	vals, err := valsOpt.MergeValues(getter.All(settings))
	if err != nil {
		return err
	}

	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return err
	}

	_, err = upgrade.Run(chart.GetReleaseName(), chartRequested, vals)
	if err != nil {
		return err
	}

	return nil
}

// GetLatestChartVersion returns the latest version from the charts directory
func GetLatestChartVersion(chartsDir string) (string, error) {
	// Read all directories in charts/
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		return "", err
	}

	// Filter directories and get their names
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}

	// Sort versions
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] < versions[j]
	})

	// Return the latest version or empty if no versions found
	if len(versions) == 0 {
		return "", fmt.Errorf("no chart versions found in %s", chartsDir)
	}

	return versions[len(versions)-1], nil
}

// GetPreviousChartVersion returns the version before the latest from the charts directory
func GetPreviousChartVersion(chartsDir string) (string, error) {
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		return "", err
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}

	if len(versions) < 2 {
		return "", fmt.Errorf("not enough versions found in %s (need at least 2)", chartsDir)
	}

	// Sort versions
	sort.Strings(versions)

	// Return the second-to-last version
	return versions[len(versions)-2], nil
}
