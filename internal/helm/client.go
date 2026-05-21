// Package helm contains a simple Helm client that implements a few actions found in the main Helm CLI.
package helm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// K3kNamespace is the default namespace for k3k resources
	K3kNamespace = "k3k-system"
	// K3kRepoName is the name of the default k3k Helm repository
	K3kRepoName = "k3k"
	// K3kRepoURL is the URL of the default k3k Helm repository
	K3kRepoURL = "https://rancher.github.io/k3k"
)

var ErrReleaseMismatch = errors.New("chart release is not matching the current build")

type Client struct {
	restClientGetter genericclioptions.RESTClientGetter
	chartRef         string
	releaseName      string
	namespace        string
	settings         *cli.EnvSettings
	repoFile         string
	repoCache        string
}

// New creates a Client using a RESTClientGetter that will be used for Helm calls to the Kubernetes API.
// The chartRef must be a repository reference in the format "repo/chart" (e.g., "k3k/k3k", "bitnami/nginx").
// Automatically adds the k3k repository (https://rancher.github.io/k3k) if not already present.
func New(clientGetter genericclioptions.RESTClientGetter, chartRef, name, namespace string) (Client, error) {
	settings := cli.New()

	if err := os.MkdirAll(settings.RepositoryCache, 0o755); err != nil {
		return Client{}, fmt.Errorf("failed to create cache directory: %w", err)
	}

	repoFile := repo.NewFile()
	if _, err := os.Stat(settings.RepositoryConfig); os.IsNotExist(err) {
		if err := repoFile.WriteFile(settings.RepositoryConfig, 0o644); err != nil {
			return Client{}, fmt.Errorf("failed to initialize repo file: %w", err)
		}
	}

	client := Client{
		restClientGetter: clientGetter,
		chartRef:         chartRef,
		releaseName:      name,
		namespace:        namespace,
		settings:         settings,
		repoFile:         settings.RepositoryConfig,
		repoCache:        settings.RepositoryCache,
	}

	// Add k3k repository by default
	if err := client.addK3kRepo(); err != nil {
		return Client{}, fmt.Errorf("failed to add k3k repository: %w", err)
	}

	return client, nil
}

// addK3kRepo adds the default k3k repository if not already present.
func (c *Client) addK3kRepo() error {
	repoFile, err := repo.LoadFile(c.repoFile)
	if err != nil {
		return fmt.Errorf("failed to load repository file: %w", err)
	}

	// Skip if already exists
	if repoFile.Has(K3kRepoName) {
		return nil
	}

	entry := &repo.Entry{
		Name: K3kRepoName,
		URL:  K3kRepoURL,
	}

	chartRepo, err := repo.NewChartRepository(entry, getter.All(c.settings))
	if err != nil {
		return fmt.Errorf("failed to create chart repository: %w", err)
	}

	chartRepo.CachePath = c.repoCache

	if _, err := chartRepo.DownloadIndexFile(); err != nil {
		return fmt.Errorf("failed to download repository index for %s: %w", K3kRepoName, err)
	}

	repoFile.Update(entry)

	if err := repoFile.WriteFile(c.repoFile, 0o644); err != nil {
		return fmt.Errorf("failed to write repository file: %w", err)
	}

	return nil
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

// InstallChart installs a Helm release from a repository chart.
func (c *Client) InstallChart(ctx context.Context, values map[string]any) error {
	logger := helmLogger(ctrl.LoggerFrom(ctx))

	var cfg action.Configuration
	if err := cfg.Init(c.restClientGetter, c.namespace, "", logger); err != nil {
		return err
	}

	install := action.NewInstall(&cfg)
	install.ReleaseName = c.releaseName
	install.Namespace = c.namespace
	install.CreateNamespace = true
	install.Wait = true
	install.Timeout = time.Minute

	chartPath, err := install.LocateChart(c.chartRef, c.settings)
	if err != nil {
		return fmt.Errorf("failed to locate chart %s: %w", c.chartRef, err)
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return err
	}

	_, err = install.Run(chart, values)

	return err
}

// AddRepository adds a Helm repository by name and URL.
func (c *Client) AddRepository(ctx context.Context, repoName, repoURL string) error {
	logger := helmLogger(ctrl.LoggerFrom(ctx))

	repoFile, err := repo.LoadFile(c.repoFile)
	if err != nil {
		return fmt.Errorf("failed to load repository file: %w", err)
	}

	if repoFile.Has(repoName) {
		logger("repository %s already exists", repoName)
		return nil
	}

	entry := &repo.Entry{
		Name: repoName,
		URL:  repoURL,
	}

	chartRepo, err := repo.NewChartRepository(entry, getter.All(c.settings))
	if err != nil {
		return fmt.Errorf("failed to create chart repository: %w", err)
	}

	chartRepo.CachePath = c.repoCache

	if _, err := chartRepo.DownloadIndexFile(); err != nil {
		return fmt.Errorf("failed to download repository index for %s: %w", repoName, err)
	}

	repoFile.Update(entry)

	if err := repoFile.WriteFile(c.repoFile, 0o644); err != nil {
		return fmt.Errorf("failed to write repository file: %w", err)
	}

	logger("successfully added repository %s", repoName)
	return nil
}

// UpdateRepositories updates the index for all configured repositories.
func (c *Client) UpdateRepositories(ctx context.Context) error {
	logger := helmLogger(ctrl.LoggerFrom(ctx))

	repoFile, err := repo.LoadFile(c.repoFile)
	if err != nil {
		return fmt.Errorf("failed to load repository file: %w", err)
	}

	var updateErrors []error
	for _, entry := range repoFile.Repositories {
		chartRepo, err := repo.NewChartRepository(entry, getter.All(c.settings))
		if err != nil {
			updateErrors = append(updateErrors, fmt.Errorf("repo %s: %w", entry.Name, err))
			continue
		}

		chartRepo.CachePath = c.repoCache

		if _, err := chartRepo.DownloadIndexFile(); err != nil {
			updateErrors = append(updateErrors, fmt.Errorf("repo %s: %w", entry.Name, err))
			continue
		}

		logger("updated repository %s", entry.Name)
	}

	if len(updateErrors) > 0 {
		return fmt.Errorf("failed to update repositories: %v", updateErrors)
	}

	return nil
}

// helmLogger wraps a logger that will write messages using an expected pattern of a fmt string followed by arguments.
func helmLogger(logger logr.Logger) action.DebugLog {
	return func(format string, v ...any) {
		logger.Info(fmt.Sprintf(format, v...))
	}
}
