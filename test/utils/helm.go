package utils

import (
	"os"
	"path/filepath"
	"time"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/kube"
	"k8s.io/client-go/rest"
)

func NewDefaultHelmActionConfig(chart Chart) (*action.Configuration, *cli.EnvSettings, error) {
	settings := cli.New()
	actionConfig := new(action.Configuration)
	err := actionConfig.Init(settings.RESTClientGetter(), chart.GetReleaseNamespace(), "secrets")
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
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, "secrets"); err != nil {
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
	install.WaitStrategy = kube.StatusWatcherStrategy
	install.WaitForJobs = true

	remote, ok := chart.(RemoteChart)
	if ok {
		install.RepoURL = remote.RepoURL
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
	uninstall.WaitStrategy = kube.LegacyStrategy
	uninstall.Timeout = time.Minute * 2

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
	upgrade.WaitStrategy = kube.StatusWatcherStrategy
	upgrade.WaitForJobs = true

	remote, ok := chart.(RemoteChart)
	if ok {
		upgrade.RepoURL = remote.RepoURL
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
