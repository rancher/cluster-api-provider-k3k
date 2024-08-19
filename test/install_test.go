package test

import (
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestProvisionExampleCluster(t *testing.T) {
	tools := []string{
		"docker",     // Container runtime.
		"kind",       // Temporary Kubernetes cluster.
		"ctlptl",     // Easily create a Kind cluster with a registry.
		"clusterctl", // Initialize the CAPI operator.
		"tilt",       // Run everything - build this operator and push to the registry.
		"kubectl",    // Wait for resources, make assertions, extract Kubeconfig, try it out.
	}
	for _, tool := range tools {
		if !isCommandAvailable(tool) {
			t.Fatalf("%s is not available in PATH", tool)
		}
	}

	t.Log("Creating a Kind registry and cluster...")
	if err := runCommand("ctlptl", "apply", "-f", "cluster.yaml"); err != nil {
		t.Fatalf("failed to create the Kind registry and cluster: %s", err)
	}
	defer cleanup(t)

	// Tilt starts a server that will block, so it needs to be started in its own goroutine.
	errChan := make(chan error)
	go func() {
		if err := runCommand("tilt", "up", "--stream", "-f", "../Tiltfile"); err != nil {
			errChan <- err
		}
	}()
	t.Log("Waiting for Tilt to start running...")
	select {
	case err := <-errChan:
		t.Errorf("failed to run Tilt up: %s", err)
		return
	case <-time.After(2 * time.Second):
	}

	t.Log("Waiting for Tilt to finish its tasks...")
	if err := runCommand("bash", "wait.sh"); err != nil {
		t.Fatalf("failed to wait for Tilt to be fully up: %s", err)
	}

	t.Log("Creating K3k example cluster resources...")
	if err := runCommand("kubectl", "apply", "-k", "../config/samples"); err != nil {
		t.Fatalf("failed to apply example resources: %s", err)
	}

	t.Log("Waiting for the CAPI-based K3k cluster to become ready...")
	if err := runCommand("kubectl", "wait", "--timeout", "5m",
		"--for", "jsonpath={.status.controlPlaneReady}",
		"--for", "jsonpath={.status.infrastructureReady}", "cluster/capicluster-sample"); err != nil {
		t.Fatalf("failed to wait for infrastructure and controlplane statuses: %s", err)
	}

	t.Log("Trying to get the secret containing the kubeconfig...")
	if err := runCommand("bash", "-c",
		"kubectl get secret k3kcontrolplane-sample-kubeconfig -o jsonpath={.data.value} | base64 -d > config.yaml"); err != nil {
		t.Fatalf("failed to find kubeconfig secret: %s", err)
	} else {
		t.Log("Saved kubeconfig in config.yaml.")
	}
}

func cleanup(t *testing.T) {
	t.Log("Cleaning up the Kind registry and cluster...")
	if err := runCommand("ctlptl", "delete", "-f", "cluster.yaml"); err != nil {
		t.Errorf("failed to delete the Kind registry and cluster: %s", err)
	}
	_ = os.Remove("config.yaml")
}

func isCommandAvailable(name string) bool {
	cmd := exec.Command("/bin/sh", "-c", "command -v "+name)
	err := cmd.Run()
	return err == nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_, _ = io.Copy(os.Stdout, stdout)
	}()
	go func() {
		_, _ = io.Copy(os.Stderr, stderr)
	}()
	return cmd.Wait()
}
