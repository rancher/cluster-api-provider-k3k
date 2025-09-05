package controlplane_test

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/rancher/cluster-api-provider-k3k/api/controlplane/v1alpha1"

	. "github.com/onsi/gomega"
)

func TestController(t *testing.T) {
	RegisterTestingT(t)

	binaryAssetsDirectory := os.Getenv("KUBEBUILDER_ASSETS")
	if binaryAssetsDirectory == "" {
		binaryAssetsDirectory = "/usr/local/kubebuilder/bin"
	}

	tmpKubebuilderDir := path.Join(os.TempDir(), "kubebuilder")
	err := os.MkdirAll(tmpKubebuilderDir, 0o755)
	Expect(err).ToNot(HaveOccurred())

	tempDir, err := os.MkdirTemp(tmpKubebuilderDir, "envtest-*")
	Expect(err).ToNot(HaveOccurred())

	err = os.CopyFS(tempDir, os.DirFS(binaryAssetsDirectory))
	Expect(err).ToNot(HaveOccurred())

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "api", "controlplane", "v1alpha1")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: tempDir,
		Scheme:                buildScheme(),
	}

	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())

	_, err = kubernetes.NewForConfig(cfg)
	Expect(err).ToNot(HaveOccurred())

	_, err = client.New(cfg, client.Options{Scheme: testEnv.Scheme})
	Expect(err).ToNot(HaveOccurred())

	manager, err := ctrl.NewManager(cfg, ctrl.Options{
		// disable the metrics server
		Metrics: metricsserver.Options{BindAddress: "0"},
		Scheme:  testEnv.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	go func() {
		t.Log("starting manager")
		startErr := manager.Start(context.Background())
		Expect(startErr).ToNot(HaveOccurred())
	}()

	t.Log("waiting for 10s")
	time.Sleep(10 * time.Second)

	t.Log("stopping testEnv")
	t.Log("manager err", testEnv.Stop())

	err = os.RemoveAll(tmpKubebuilderDir)
	Expect(err).ToNot(HaveOccurred())
}

func buildScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()

	err := clientgoscheme.AddToScheme(scheme)
	Expect(err).ToNot(HaveOccurred())
	err = v1alpha1.AddToScheme(scheme)
	Expect(err).ToNot(HaveOccurred())

	return scheme
}
