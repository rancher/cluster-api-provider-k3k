// Package helm contains a simple Helm client that implements a few actions found in the main Helm CLI.
package helm

import (
	"context"
	"errors"
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Client struct {
	restClientGetter genericclioptions.RESTClientGetter
	chartPath        string
	releaseName      string
	namespace        string
}

var ErrReleaseOutdated = errors.New("release is of an outdated chart version")

// New creates a Client using a RESTClientGetter that will be used for Helm calls to the Kubernetes API.
// It also takes the relative path that must have the desired chart, release name and namespace.
func New(clientGetter genericclioptions.RESTClientGetter, chartPath, name, namespace string) Client {
	return Client{
		restClientGetter: clientGetter,
		chartPath:        chartPath,
		releaseName:      name,
		namespace:        namespace,
	}
}

// ReleasePresent tries to get a Helm release specified by name, version, and namespace in which it's deployed.
func (c *Client) ReleasePresent(ctx context.Context, version string) error {
	var cfg action.Configuration
	if err := cfg.Init(c.restClientGetter, c.namespace, "", helmLogger(ctrl.LoggerFrom(ctx).Info)); err != nil {
		return err
	}
	get := action.NewGet(&cfg)
	release, err := get.Run(c.releaseName)
	if err != nil {
		return err
	}
	if release.Chart.Metadata.Version != version {
		return ErrReleaseOutdated
	}
	return nil
}

// DeployLocalChart creates a Helm release from a chart that exists on the local filesystem.
func (c *Client) DeployLocalChart(ctx context.Context, values map[string]any) error {
	chart, err := loader.Load(c.chartPath)
	if err != nil {
		return err
	}
	var cfg action.Configuration
	if err := cfg.Init(c.restClientGetter, c.namespace, "", helmLogger(ctrl.LoggerFrom(ctx).Info)); err != nil {
		return err
	}
	install := action.NewInstall(&cfg)
	install.ReleaseName = c.releaseName
	install.Namespace = c.namespace
	install.CreateNamespace = true
	if _, err := install.Run(chart, values); err != nil {
		return err
	}
	return nil
}

// UpgradeLocalChart upgrades a Helm release using a chart that exists on the local filesystem.
// The Helm SDK doesn't allow to use a single action to either install or upgrade, unlike the CLI.
func (c *Client) UpgradeLocalChart(ctx context.Context, values map[string]any) error {
	chart, err := loader.Load(c.chartPath)
	if err != nil {
		return err
	}
	var cfg action.Configuration
	if err := cfg.Init(c.restClientGetter, c.namespace, "", helmLogger(ctrl.LoggerFrom(ctx).Info)); err != nil {
		return err
	}
	upgrade := action.NewUpgrade(&cfg)
	upgrade.Namespace = c.namespace
	upgrade.ReuseValues = true
	if _, err := upgrade.Run(c.releaseName, chart, values); err != nil {
		return err
	}
	return nil
}

// helmLogger wraps a logger that will write messages using an expected pattern of a fmt string followed by arguments.
func helmLogger(log action.DebugLog) action.DebugLog {
	return func(format string, v ...interface{}) {
		log(fmt.Sprintf(format, v...))
	}
}
