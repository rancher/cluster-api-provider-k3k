package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/rancher-sandbox/cluster-api-provider-k3k/internal/helm"
	upstream "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	"github.com/rancher/k3k/pkg/controller/kubeconfig"
	"github.com/rancher/k3k/pkg/controller/util"
	"helm.sh/helm/v3/pkg/storage/driver"
	apiError "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// reconcileUpstreamCluster creates/updates the k3k cluster that the k3k controllers will recognize.
// It doesn't remove clusters if the controlPlane is being deleted.
func reconcileUpstreamCluster(ctx context.Context, scope *scope) (*upstream.Cluster, error) {
	controlPlane := scope.k3kControlPlane
	hostClient := scope.hostClient
	log := ctrl.LoggerFrom(ctx)

	clusters, err := getUpstreamClusters(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to get current clusters for controlPlane %s/%s: %w", controlPlane.Namespace, controlPlane.Name, err)
	}
	spec := upstream.ClusterSpec{
		Version:     controlPlane.Spec.Version,
		Servers:     controlPlane.Spec.Servers,
		Agents:      controlPlane.Spec.Agents,
		Token:       controlPlane.Spec.Token,
		ClusterCIDR: controlPlane.Spec.ClusterCIDR,
		ServiceCIDR: controlPlane.Spec.ServiceCIDR,
		ClusterDNS:  controlPlane.Spec.ClusterDNS,
		ServerArgs:  controlPlane.Spec.ServerArgs,
		AgentArgs:   controlPlane.Spec.AgentArgs,
		TLSSANs:     controlPlane.Spec.TLSSANs,
		Addons:      controlPlane.Spec.Addons,
		Persistence: controlPlane.Spec.Persistence,
		Expose:      controlPlane.Spec.Expose,
	}
	if len(clusters) > 1 {
		var names []string
		for i := range clusters {
			names = append(names, clusters[i].Name)
		}
		joinedNames := strings.Join(names, ", ")
		log.Error(fmt.Errorf("controlplane %s owns two or more clusters: %s, refusing to process further", controlPlane.Name, joinedNames), "need to delete upstream clusters")
		if err := deleteUpstreamClusters(ctx, scope); err != nil {
			return nil, fmt.Errorf("failed to delete upstream clusters owned by a single controPlane %s: %w", controlPlane.Name, err)
		}
		return nil, fmt.Errorf("reprovisioning needed")
	}
	if len(clusters) == 0 {
		// since the upstream cluster type isn't namespaced but we are, we can't use owner refs to control deletion of the cluster
		upstreamCluster := upstream.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: controlPlane.Name + "-",
				Labels: map[string]string{
					ownerNameLabel:      controlPlane.Name,
					ownerNamespaceLabel: controlPlane.Namespace,
				},
			},
			Spec: spec,
		}
		err = hostClient.Create(ctx, &upstreamCluster)
		if err != nil {
			return nil, fmt.Errorf("unable to create cluster for controlPlane %s: %w", controlPlane.Name, err)
		}
		return &upstreamCluster, nil
	}
	// at this point we have exactly one cluster
	currentCluster := clusters[0].DeepCopy()
	if reflect.DeepEqual(currentCluster.Spec, spec) {
		return currentCluster, nil
	}
	currentCluster.Spec = spec
	err = hostClient.Update(ctx, currentCluster)
	if err != nil {
		return nil, fmt.Errorf("unable to update cluster for controlPlane %s: %w", controlPlane.Name, err)
	}
	return currentCluster, nil
}

// deleteUpstreamClusters deletes all upstream clusters associated with this controlPlane.
func deleteUpstreamClusters(ctx context.Context, scope *scope) error {
	hostClient := scope.hostClient
	controlPlane := scope.k3kControlPlane
	log := ctrl.LoggerFrom(ctx)

	clusters, err := getUpstreamClusters(ctx, scope)
	if err != nil {
		return fmt.Errorf("unable to get current clusters for controlPlane %s/%s: %w", controlPlane.Namespace, controlPlane.Name, err)
	}
	var deleteErrs []error
	for i := range clusters {
		if err := hostClient.Delete(ctx, &clusters[i]); err != nil && !apiError.IsNotFound(err) {
			log.Error(err, "failed to delete upstream cluster "+clusters[i].Name)
			deleteErrs = append(deleteErrs, err)
		}
	}
	if err := errors.Join(deleteErrs...); err != nil {
		return fmt.Errorf("failed to delete some upstream clusters: %w", err)
	}

	return nil
}

func getUpstreamClusters(ctx context.Context, scope *scope) ([]upstream.Cluster, error) {
	controlPlane := scope.k3kControlPlane
	hostClient := scope.hostClient
	var currentClusters upstream.ClusterList
	ownerNameRequirement, err := labels.NewRequirement(ownerNameLabel, selection.Equals, []string{controlPlane.Name})
	if err != nil {
		return nil, fmt.Errorf("unable to form label for controlPlane %s: %w", controlPlane.Name, err)
	}
	ownerNamespaceRequirement, err := labels.NewRequirement(ownerNamespaceLabel, selection.Equals, []string{controlPlane.Namespace})
	if err != nil {
		return nil, fmt.Errorf("unable to form label for controlPlane %s: %w", controlPlane.Name, err)
	}
	selector := labels.NewSelector().Add(*ownerNameRequirement).Add(*ownerNamespaceRequirement)
	err = hostClient.List(ctx, &currentClusters, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("unable to list current clusters %w", err)
	}
	return currentClusters.Items, nil
}

// ensureUpstreamChart tries to install a release of the upstream K3K chart or upgrade it if there is a new version.
func ensureUpstreamChart(ctx context.Context, scope *scope, version string) error {
	log := ctrl.LoggerFrom(ctx)
	helmClient := scope.hostHelmClient
	if err := helmClient.ReleasePresent(ctx, version); err != nil {
		values := map[string]any{}
		if errors.Is(err, driver.ErrReleaseNotFound) {
			if err := helmClient.DeployLocalChart(ctx, values); err != nil {
				return fmt.Errorf("failed to deploy a local K3K release: %w", err)
			}
			log.Info("Successfully deployed the upstream K3K release.")
			return nil
		}
		if errors.Is(err, helm.ErrReleaseOutdated) {
			if err := helmClient.UpgradeLocalChart(ctx, values); err != nil {
				return fmt.Errorf("failed to upgrade a local K3K release: %w", err)
			}
			log.Info("Successfully upgraded the upstream K3K release.")
			return nil
		}
		return fmt.Errorf("couldn't check for the presence of a K3K release: %w", err)
	}
	return nil
}

// getKubeconfig retrieves the kubeconfig for the given upstream K3k cluster.
func getKubeconfig(ctx context.Context, scope *scope, upstreamCluster *upstream.Cluster) (*clientcmdapi.Config, error) {
	// TODO: We set the expiry to one year here, but we need to continually keep this up to date
	hostClient := scope.hostClient
	cfg := kubeconfig.KubeConfig{
		CN:         util.AdminCommonName,
		ORG:        []string{user.SystemPrivilegedGroup},
		ExpiryDate: time.Hour * 24 * 365,
	}
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get current config to derive hostname: %w", err)
	}
	u, err := url.Parse(restConfig.Host)
	if err != nil {
		return nil, fmt.Errorf("unable to extract URL from specificed hostname: %s, %w", restConfig.Host, err)
	}
	rawKubeConfig, err := cfg.Extract(ctx, hostClient, upstreamCluster, u.Hostname())
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve raw kubeconfig data: %w", err)
	}
	return clientcmd.Load(rawKubeConfig)
}
