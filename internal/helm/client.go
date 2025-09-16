// Package helm contains a simple Helm client that implements a few actions found in the main Helm CLI.
package helm

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	ctrl "sigs.k8s.io/controller-runtime"
)

var ErrReleaseMismatch = errors.New("chart release is not matching the current build")

type Client struct {
	restClientGetter genericclioptions.RESTClientGetter
	chartPath        string
	releaseName      string
	namespace        string
}

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

// GetRelease tries to get a Helm release specified by name, version, and namespace in which it's deployed.
func (c *Client) GetRelease(ctx context.Context) (*release.Release, error) {
	logger := helmLogger(ctrl.LoggerFrom(ctx))

	var cfg action.Configuration
	if err := cfg.Init(c.restClientGetter, c.namespace, "", logger); err != nil {
		return nil, err
	}

	get := action.NewGet(&cfg)

	return get.Run(c.releaseName)
}

// DeployLocalChart creates a Helm release from a chart that exists on the local filesystem.
func (c *Client) DeployLocalChart(ctx context.Context, values map[string]any) error {
	logger := helmLogger(ctrl.LoggerFrom(ctx))

	var cfg action.Configuration
	if err := cfg.Init(c.restClientGetter, c.namespace, "", logger); err != nil {
		return err
	}

	install := action.NewInstall(&cfg)
	install.ReleaseName = c.releaseName
	install.Namespace = c.namespace
	install.CreateNamespace = true

	chart, err := loader.Load(c.chartPath)
	if err != nil {
		return err
	}

	_, err = install.Run(chart, values)

	return err
}

// UpgradeLocalChart upgrades a Helm release using a chart that exists on the local filesystem.
// The Helm SDK doesn't allow to use a single action to either install or upgrade, unlike the CLI.
func (c *Client) UpgradeLocalChart(ctx context.Context, values map[string]any) error {
	logger := helmLogger(ctrl.LoggerFrom(ctx))

	var cfg action.Configuration
	if err := cfg.Init(c.restClientGetter, c.namespace, "", logger); err != nil {
		return err
	}

	upgrade := action.NewUpgrade(&cfg)
	upgrade.Namespace = c.namespace
	upgrade.ReuseValues = true

	chart, err := loader.Load(c.chartPath)
	if err != nil {
		return err
	}

	_, err = upgrade.Run(c.releaseName, chart, values)

	return err
}

// helmLogger wraps a logger that will write messages using an expected pattern of a fmt string followed by arguments.
func helmLogger(logger logr.Logger) action.DebugLog {
	return func(format string, v ...any) {
		logger.Info(fmt.Sprintf(format, v...))
	}
}
