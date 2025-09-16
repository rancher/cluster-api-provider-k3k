package e2e_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	e2eConfig             *clusterctl.E2EConfig
	clusterctlLogFolder   string
	clusterctlConfigPath  string //nolint:revive
	bootstrapClusterProxy framework.ClusterProxy
	scheme                *runtime.Scheme
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}

var _ = BeforeSuite(func() {
	scheme = runtime.NewScheme()
	framework.TryAddDefaultSchemes(scheme)

	pwd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())

	e2eConfig = clusterctl.LoadE2EConfig(context.Background(),
		clusterctl.LoadE2EConfigInput{
			ConfigPath: filepath.Join(pwd, "clusterctl.yaml"),
		},
	)
	Expect(e2eConfig).ToNot(BeNil(), "Failed to load E2E config")

	repoPath, err := filepath.Abs("./repository")
	Expect(err).ToNot(HaveOccurred())

	Expect(os.MkdirAll(repoPath, os.ModePerm)).To(Succeed())

	// CreateRepository expects a metadata.yaml file for each provider.
	// The upstream CAPI releases don't have this, so we create a minimal one.
	for _, provider := range e2eConfig.Providers {
		for _, version := range provider.Versions {
			providerLabel := clusterctlv1.ManifestLabel(provider.Name, clusterctlv1.ProviderType(provider.Type))
			providerPath := filepath.Join(repoPath, providerLabel, version.Name)
			Expect(os.MkdirAll(providerPath, os.ModePerm)).To(Succeed())

			var major, minor int
			if provider.Name == "k3k" {
				major = 0
				minor = 0
			} else {
				major = 1
				minor = 10
			}

			metadataFile := filepath.Join(providerPath, "metadata.yaml")
			metadataContent := []byte(fmt.Sprintf(`apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
kind: Metadata
releaseSeries:
  - major: %d
    minor: %d
    contract: v1beta1`, major, minor))
			Expect(os.WriteFile(metadataFile, metadataContent, 0o600)).To(Succeed())
		}
	}

	clusterctlConfigPath = clusterctl.CreateRepository(context.Background(), clusterctl.CreateRepositoryInput{
		E2EConfig:        e2eConfig,
		RepositoryFolder: repoPath,
	})

	kubeconfig := os.Getenv("KUBECONFIG")
	GinkgoLogr.Info("Loading kubeconfig", "kubeconfig", kubeconfig)

	bootstrapClusterProxy = framework.NewClusterProxy("bootstrap", kubeconfig, scheme)
	clusterctlLogFolder = filepath.Join("./", "logs", bootstrapClusterProxy.GetName())

	By("Initializing the management cluster")
	clusterctl.InitManagementClusterAndWatchControllerLogs(context.Background(), clusterctl.InitManagementClusterAndWatchControllerLogsInput{
		ClusterProxy:            bootstrapClusterProxy,
		ClusterctlConfigPath:    clusterctlConfigPath,
		CoreProvider:            config.ClusterAPIProviderName,
		BootstrapProviders:      []string{config.KubeadmBootstrapProviderName},
		ControlPlaneProviders:   []string{config.KubeadmControlPlaneProviderName},
		InfrastructureProviders: e2eConfig.InfrastructureProviders(),
		LogFolder:               clusterctlLogFolder,
	})
})

var _ = AfterSuite(func() {
	By("Tearing down the management cluster")
	bootstrapClusterProxy.Dispose(context.Background())
})

var _ = When("install", func() {
	It("ok", func() {
		By("eyah")
	})
})
