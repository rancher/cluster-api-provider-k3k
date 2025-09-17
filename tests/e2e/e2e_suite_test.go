package e2e_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/stdr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"

	ctrl "sigs.k8s.io/controller-runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	artifactFolder           string
	e2eConfig                *clusterctl.E2EConfig
	clusterctlLogFolder      string
	clusterctlConfigPath     string //nolint:revive
	bootstrapClusterProvider bootstrap.ClusterProvider
	bootstrapClusterProxy    framework.ClusterProxy
)

func TestE2e(t *testing.T) {
	ctrl.SetLogger(stdr.New(log.New(os.Stdout, "", log.LstdFlags)))
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}

// Using a SynchronizedBeforeSuite for controlling how to create resources shared across ParallelNodes (~ginkgo threads).
// The local clusterctl repository & the bootstrap cluster are created once and shared across all the tests.
var _ = SynchronizedBeforeSuite(func() []byte {
	var err error
	ctx := context.Background()

	configPath, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())

	configPath = filepath.Join(configPath, "clusterctl.yaml")
	Expect(configPath).To(BeAnExistingFile(), "Invalid test suite argument. e2e.config should be an existing file.")

	By("Loading the e2e test configuration from " + configPath)
	e2eConfig = clusterctl.LoadE2EConfig(ctx, clusterctl.LoadE2EConfigInput{ConfigPath: configPath})
	Expect(e2eConfig).NotTo(BeNil(), "Failed to load E2E config from %s", configPath)

	artifactFolder, err = filepath.Abs(".")
	Expect(err).ToNot(HaveOccurred())

	repoPath, err := filepath.Abs("./repository")
	Expect(err).ToNot(HaveOccurred())

	createRepositoryInput := clusterctl.CreateRepositoryInput{
		E2EConfig:        e2eConfig,
		RepositoryFolder: repoPath,
	}

	clusterctlConfigPath = clusterctl.CreateRepository(ctx, createRepositoryInput)
	Expect(clusterctlConfigPath).To(BeAnExistingFile(), "The clusterctl config file does not exists in the local repository: "+repoPath)

	By("Setting up the bootstrap cluster")
	bootstrapClusterProvider, bootstrapClusterProxy = setupBootstrapCluster(ctx, e2eConfig, false)

	clusterctlLogFolder = filepath.Join(artifactFolder, "clusters", bootstrapClusterProxy.GetName())

	By("Initializing the bootstrap cluster")
	initBootstrapCluster(ctx, bootstrapClusterProxy, e2eConfig, clusterctlConfigPath, artifactFolder)

	// encode the e2e config into the byte array.
	var configBuf bytes.Buffer
	enc := gob.NewEncoder(&configBuf)
	Expect(enc.Encode(e2eConfig)).To(Succeed())
	configStr := base64.StdEncoding.EncodeToString(configBuf.Bytes())

	return []byte(
		strings.Join([]string{
			artifactFolder,
			clusterctlConfigPath,
			configStr,
			bootstrapClusterProxy.GetKubeconfigPath(),
		}, ","),
	)
}, func(data []byte) {
	// Before each ParallelNode.

	parts := strings.Split(string(data), ",")
	Expect(parts).To(HaveLen(4))

	artifactFolder = parts[0]
	clusterctlConfigPath = parts[1]

	// Decode the e2e config
	configBytes, err := base64.StdEncoding.DecodeString(parts[2])
	Expect(err).NotTo(HaveOccurred())
	buf := bytes.NewBuffer(configBytes)
	dec := gob.NewDecoder(buf)
	Expect(dec.Decode(&e2eConfig)).To(Succeed())

	kubeconfigPath := parts[3]
	By(kubeconfigPath)
})

// Using a SynchronizedAfterSuite for controlling how to delete resources shared across ParallelNodes (~ginkgo threads).
// The bootstrap cluster is shared across all the tests, so it should be deleted only after all ParallelNodes completes.
// The local clusterctl repository is preserved like everything else created into the artifact folder.
var _ = SynchronizedAfterSuite(afterEachParallelNode, afterAllParallelNodes)

// After each ParallelNode.
func afterEachParallelNode() {}

// After all ParallelNodes.
func afterAllParallelNodes() {
	By("Tearing down the management cluster")
	tearDown(context.Background(), bootstrapClusterProvider, bootstrapClusterProxy)
}

func setupBootstrapCluster(ctx context.Context, config *clusterctl.E2EConfig, useExistingCluster bool) (bootstrap.ClusterProvider, framework.ClusterProxy) {
	var clusterProvider bootstrap.ClusterProvider
	var kubeconfigPath string

	if !useExistingCluster {
		clusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(ctx, bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
			Name:               config.ManagementClusterName,
			RequiresDockerSock: config.HasDockerProvider(),
			Images:             config.Images,
		})
		Expect(clusterProvider).NotTo(BeNil(), "Failed to create a bootstrap cluster")

		kubeconfigPath = clusterProvider.GetKubeconfigPath()
		Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the bootstrap cluster")
	} else {
		kubeconfigPath = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		// @sonasingh46: Workaround for testing workload identity.
		// Loading image for already created cluster
		imagesInput := bootstrap.LoadImagesToKindClusterInput{
			Name:   "capz-e2e",
			Images: config.Images,
		}
		err := bootstrap.LoadImagesToKindCluster(ctx, imagesInput)
		Expect(err).NotTo(HaveOccurred(), "Failed to load images to the bootstrap cluster: %s", err)
	}

	scheme := runtime.NewScheme()
	framework.TryAddDefaultSchemes(scheme)
	clusterProxy := framework.NewClusterProxy("bootstrap", kubeconfigPath, scheme)
	Expect(clusterProxy).NotTo(BeNil(), "Failed to get a bootstrap cluster proxy")

	return clusterProvider, clusterProxy
}

func initBootstrapCluster(ctx context.Context, bootstrapClusterProxy framework.ClusterProxy, config *clusterctl.E2EConfig, clusterctlConfig, artifactFolder string) {
	clusterctl.InitManagementClusterAndWatchControllerLogs(ctx, clusterctl.InitManagementClusterAndWatchControllerLogsInput{
		ClusterProxy:            bootstrapClusterProxy,
		ClusterctlConfigPath:    clusterctlConfig,
		InfrastructureProviders: config.InfrastructureProviders(),
		AddonProviders:          config.AddonProviders(),
		LogFolder:               clusterctlLogFolder,
	}, config.GetIntervals(bootstrapClusterProxy.GetName(), "wait-controllers")...)
}

func tearDown(ctx context.Context, bootstrapClusterProvider bootstrap.ClusterProvider, bootstrapClusterProxy framework.ClusterProxy) {
	if bootstrapClusterProxy != nil {
		bootstrapClusterProxy.Dispose(ctx)
	}
	if bootstrapClusterProvider != nil {
		bootstrapClusterProvider.Dispose(ctx)
	}
}

/////////////////////////////

// var _ = BeforeSuite(func() {
// 	scheme = runtime.NewScheme()
// 	framework.TryAddDefaultSchemes(scheme)

// 	// pwd, err := os.Getwd()
// 	// Expect(err).ToNot(HaveOccurred())

// 	e2eConfig = clusterctl.LoadE2EConfig(context.Background(),
// 		clusterctl.LoadE2EConfigInput{
// 			// ConfigPath: filepath.Join(pwd, "clusterctl.yaml"),
// 			ConfigPath: "clusterctl.yaml",
// 		},
// 	)
// 	Expect(e2eConfig).ToNot(BeNil(), "Failed to load E2E config")

// 	repoPath, err := filepath.Abs("./repository")
// 	Expect(err).ToNot(HaveOccurred())

// 	Expect(os.MkdirAll(repoPath, os.ModePerm)).To(Succeed())

// 	// CreateRepository expects a metadata.yaml file for each provider.
// 	// The upstream CAPI releases don't have this, so we create a minimal one.
// 	for _, provider := range e2eConfig.Providers {
// 		for _, version := range provider.Versions {
// 			providerLabel := clusterctlv1.ManifestLabel(provider.Name, clusterctlv1.ProviderType(provider.Type))
// 			providerPath := filepath.Join(repoPath, providerLabel, version.Name)
// 			Expect(os.MkdirAll(providerPath, os.ModePerm)).To(Succeed())

// 			// The k3k provider already has a metadata.yaml, so we only create it for the others.
// 			if provider.Name != "k3k" {
// 				metadataFile := filepath.Join(providerPath, "metadata.yaml")
// 				metadataContent := []byte(fmt.Sprintf(`apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
// kind: Metadata
// releaseSeries:
//   - major: 1
//     minor: 10
//     contract: v1beta1`))
// 				Expect(os.WriteFile(metadataFile, metadataContent, 0o600)).To(Succeed())
// 			}
// 		}
// 	}

// 	// Also, ensure the k3k provider's metadata.yaml is copied to the repository.
// 	// k3kProvider := e2eConfig.GetProvider("k3k")
// 	// k3kVersion := k3kProvider.Versions[0]
// 	// k3kPath := filepath.Join(repoPath, clusterctlv1.ManifestLabel("k3k", clusterctlv1.InfrastructureProviderType), k3kVersion.Name)
// 	// Expect(os.WriteFile(filepath.Join(k3kPath, "metadata.yaml"), []byte(e2eConfig.GetFile(k3kVersion.Files[0].SourcePath)), 0o600)).To(Succeed())

// 	clusterctlConfigPath = clusterctl.CreateRepository(context.Background(), clusterctl.CreateRepositoryInput{
// 		E2EConfig:        e2eConfig,
// 		RepositoryFolder: repoPath,
// 	})

// 	kubeconfig := os.Getenv("KUBECONFIG")
// 	GinkgoLogr.Info("Loading kubeconfig", "kubeconfig", kubeconfig)

// 	bootstrapClusterProxy = framework.NewClusterProxy("bootstrap", kubeconfig, scheme)
// 	clusterctlLogFolder = filepath.Join("./", "logs", bootstrapClusterProxy.GetName())

// 	By("Initializing the management cluster")
// 	clusterctl.InitManagementClusterAndWatchControllerLogs(context.Background(), clusterctl.InitManagementClusterAndWatchControllerLogsInput{
// 		ClusterProxy:            bootstrapClusterProxy,
// 		ClusterctlConfigPath:    clusterctlConfigPath,
// 		CoreProvider:            config.ClusterAPIProviderName,
// 		BootstrapProviders:      []string{config.KubeadmBootstrapProviderName},
// 		ControlPlaneProviders:   []string{config.KubeadmControlPlaneProviderName},
// 		InfrastructureProviders: e2eConfig.InfrastructureProviders(),
// 		LogFolder:               clusterctlLogFolder,
// 	})
// })

// var _ = AfterSuite(func() {
// 	By("Tearing down the management cluster")
// 	bootstrapClusterProxy.Dispose(context.Background())
// })

var _ = When("install", func() {
	It("ok", func() {
		By("eyah")
	})
})
