package e2e_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/stdr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"

	utilrand "k8s.io/apimachinery/pkg/util/rand"
	ctrl "sigs.k8s.io/controller-runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	configPath         string
	artifactFolder     string
	useExistingCluster bool
	kindClusterName    string

	e2eConfig            *clusterctl.E2EConfig
	clusterctlConfigPath string

	bootstrapClusterProvider bootstrap.ClusterProvider
	bootstrapClusterProxy    framework.ClusterProxy
)

func init() {
	flag.StringVar(&configPath, "e2e.config", "clusterctl.yaml", "path to the e2e config file")
	flag.StringVar(&artifactFolder, "e2e.artifacts-folder", "_artifacts", "folder where e2e test artifact should be stored")
	flag.BoolVar(&useExistingCluster, "e2e.use-existing-cluster", false, "if true, the test uses the current cluster instead of creating a new one (default discovery rules apply)")
	flag.StringVar(&kindClusterName, "e2e.kind-cluster-name", "", "name of the Kind cluster used to load the images")
}

func TestE2e(t *testing.T) {
	ctrl.SetLogger(stdr.New(log.New(os.Stdout, "", log.LstdFlags)))
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}

// Using a SynchronizedBeforeSuite for controlling how to create resources shared across ParallelNodes (~ginkgo threads).
// The local clusterctl repository & the bootstrap cluster are created once and shared across all the tests.
var _ = SynchronizedBeforeSuite(beforeAllParallelNodes, beforeEachParallelNode)

// After all ParallelNodes.
func beforeAllParallelNodes() []byte {
	var err error
	ctx := context.Background()

	Expect(configPath).To(BeAnExistingFile(), "Invalid test suite argument. e2e.config should be an existing file.")
	artifactFolder, err = filepath.Abs(artifactFolder)
	Expect(err).NotTo(HaveOccurred(), "Invalid test suite argument. e2e.artifacts-folder should be a valid path.")

	By("Loading the e2e test configuration from " + configPath)
	By("Storing artifacts in " + artifactFolder)

	clusterctlConfigPath := createClusterctlLocalRepository(ctx, configPath, artifactFolder)

	if kindClusterName != "" {
		e2eConfig.ManagementClusterName = kindClusterName
	}

	By("Setting up the bootstrap cluster")
	bootstrapClusterProvider, bootstrapClusterProxy = setupBootstrapCluster(ctx, e2eConfig, useExistingCluster)

	By("Initializing the bootstrap cluster")
	initBootstrapCluster(ctx, bootstrapClusterProxy, e2eConfig, clusterctlConfigPath)

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
}

// Before each ParallelNode.
func beforeEachParallelNode(data []byte) {
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
}

func createClusterctlLocalRepository(ctx context.Context, configPath string, artifactFolder string) string {
	e2eConfig = clusterctl.LoadE2EConfig(ctx, clusterctl.LoadE2EConfigInput{ConfigPath: configPath})
	Expect(e2eConfig).NotTo(BeNil(), "Failed to load E2E config from %s", configPath)

	// set default intervals
	e2eConfig.Intervals = map[string][]string{}
	e2eConfig.Intervals["default/wait-cluster"] = []string{"3m"}

	repositoryFolder := filepath.Join(artifactFolder, "repository")

	createRepositoryInput := clusterctl.CreateRepositoryInput{
		E2EConfig:        e2eConfig,
		RepositoryFolder: repositoryFolder,
	}

	clusterctlConfigPath = clusterctl.CreateRepository(ctx, createRepositoryInput)
	Expect(clusterctlConfigPath).To(BeAnExistingFile(), "The clusterctl config file does not exists in the local repository: "+repositoryFolder)

	return clusterctlConfigPath
}

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

	if useExistingCluster {
		By("Reusing existing Kind cluster: " + config.ManagementClusterName)

		kubeconfigPath = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()

		// Loading image for already created cluster
		imagesInput := bootstrap.LoadImagesToKindClusterInput{
			Name:   config.ManagementClusterName,
			Images: config.Images,
		}
		err := bootstrap.LoadImagesToKindCluster(ctx, imagesInput)
		Expect(err).NotTo(HaveOccurred(), "Failed to load images to the bootstrap cluster: %s", err)
	} else {
		By("Creating a new Kind cluster")

		clusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(ctx, bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
			Name:               config.ManagementClusterName,
			RequiresDockerSock: config.HasDockerProvider(),
			Images:             config.Images,
		})
		Expect(clusterProvider).NotTo(BeNil(), "Failed to create a bootstrap cluster")

		kubeconfigPath = clusterProvider.GetKubeconfigPath()
		Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the bootstrap cluster")
	}

	scheme := runtime.NewScheme()
	framework.TryAddDefaultSchemes(scheme)
	clusterProxy := framework.NewClusterProxy("bootstrap", kubeconfigPath, scheme)
	Expect(clusterProxy).NotTo(BeNil(), "Failed to get a bootstrap cluster proxy")

	return clusterProvider, clusterProxy
}

func initBootstrapCluster(ctx context.Context, bootstrapClusterProxy framework.ClusterProxy, config *clusterctl.E2EConfig, clusterctlConfig string) {
	logFolder := filepath.Join(artifactFolder, "clusters", bootstrapClusterProxy.GetName())

	clusterctl.InitManagementClusterAndWatchControllerLogs(ctx, clusterctl.InitManagementClusterAndWatchControllerLogsInput{
		ClusterProxy:            bootstrapClusterProxy,
		ClusterctlConfigPath:    clusterctlConfig,
		InfrastructureProviders: config.InfrastructureProviders(),
		AddonProviders:          config.AddonProviders(),
		LogFolder:               logFolder,
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

func noOpWaiter(ctx context.Context, input clusterctl.ApplyCustomClusterTemplateAndWaitInput, result *clusterctl.ApplyCustomClusterTemplateAndWaitResult) {
}

var _ = When("creating a Cluster from the cluster-template", func() {
	It("works", func() {
		specName := "cluster-template"

		ctx := context.Background()
		logFolder := filepath.Join(artifactFolder, "clusters", bootstrapClusterProxy.GetName())

		By("Creating a namespace for the test")

		namespace, cancelWatches := framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   bootstrapClusterProxy.GetClient(),
			ClientSet: bootstrapClusterProxy.GetClientSet(),
			Name:      specName,
			LogFolder: logFolder,
		})

		defer cancelWatches()
		defer framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
			Deleter: bootstrapClusterProxy.GetClient(),
			Name:    namespace.Name,
		})

		input := clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: bootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				Namespace:                namespace.Name,
				ClusterName:              "cluster-" + utilrand.String(5),
				ClusterctlConfigPath:     clusterctlConfigPath,
				KubeconfigPath:           bootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   e2eConfig.GetVariableOrEmpty("INFRASTRUCTURE_PROVIDER"),
				Flavor:                   e2eConfig.GetVariableOrEmpty("CONTROL_PLANE_FLAVOR"),
				KubernetesVersion:        e2eConfig.GetVariableOrEmpty("KUBERNETES_VERSION"),
				ControlPlaneMachineCount: ptr.To[int64](1),
				WorkerMachineCount:       ptr.To[int64](1),
				LogFolder:                logFolder,
			},
			WaitForClusterIntervals: e2eConfig.GetIntervals(specName, "wait-cluster"),
			ControlPlaneWaiters: clusterctl.ControlPlaneWaiters{
				WaitForControlPlaneInitialized:   noOpWaiter,
				WaitForControlPlaneMachinesReady: noOpWaiter,
			},
		}

		var result clusterctl.ApplyClusterTemplateAndWaitResult
		clusterctl.ApplyClusterTemplateAndWait(ctx, input, &result)

		Expect(result.Cluster).ToNot(BeNil(), "The returned Cluster object should not be nil")
	})
})
