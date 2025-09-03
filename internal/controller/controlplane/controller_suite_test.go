package controlplane_test

import (
	"context"
	"fmt"
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
)

func TestController(t *testing.T) {
	binaryAssetsDirectory := os.Getenv("KUBEBUILDER_ASSETS")
	if binaryAssetsDirectory == "" {
		binaryAssetsDirectory = "/usr/local/kubebuilder/bin"
	}

	tmpKubebuilderDir := path.Join(os.TempDir(), "kubebuilder")
	os.Mkdir(tmpKubebuilderDir, 0o755)
	tempDir, err := os.MkdirTemp(tmpKubebuilderDir, "envtest-*")
	fmt.Println(err)

	err = os.CopyFS(tempDir, os.DirFS(binaryAssetsDirectory))

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "charts", "k3k", "crds")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: tempDir,
		Scheme:                buildScheme(),
	}

	cfg, err := testEnv.Start()
	_, err = kubernetes.NewForConfig(cfg)
	_, err = client.New(cfg, client.Options{Scheme: testEnv.Scheme})

	manager, err := ctrl.NewManager(cfg, ctrl.Options{
		// disable the metrics server
		Metrics: metricsserver.Options{BindAddress: "0"},
		Scheme:  testEnv.Scheme,
	})

	go func() {
		t.Log("starting manager")
		t.Log("manager err", manager.Start(context.Background()))
	}()

	t.Log("waiting for 10s")
	time.Sleep(10 * time.Second)

	t.Log("stopping testEnv")
	t.Log("manager err", testEnv.Stop())

	//////

	err = os.RemoveAll(tmpKubebuilderDir)
}

func buildScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()

	clientgoscheme.AddToScheme(scheme)
	v1alpha1.AddToScheme(scheme)

	return scheme
}
