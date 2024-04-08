/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	upstream "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	"github.com/rancher/k3k/pkg/controller/kubeconfig"
	"github.com/rancher/k3k/pkg/controller/util"
	"helm.sh/helm/v3/pkg/storage/driver"
	v1 "k8s.io/api/core/v1"
	apiError "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/retry"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capiutil "sigs.k8s.io/cluster-api/util"
	capiKubeconfig "sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-k3k/api/controlplane/v1alpha1"
	infrastructurev1 "github.com/rancher-sandbox/cluster-api-provider-k3k/api/infrastructure/v1alpha1"
	"github.com/rancher-sandbox/cluster-api-provider-k3k/internal/helm"
)

const (
	ownerNameLabel      = "app.cattle.io/owner-name"
	ownerNamespaceLabel = "app.cattle.io/owner-ns"
	finalizer           = "app.cattle.io/upstream-remove"

	k3kControlPlaneKind = "K3kControlPlane"
)

// K3kControlPlaneReconciler reconciles a K3kControlPlane object
type K3kControlPlaneReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Helm       helm.Client
	K3KVersion string
}

// SetupWithManager sets up the controller with the Manager.
func (r *K3kControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	log := ctrl.LoggerFrom(ctx)

	k3kControlPlane := &controlplanev1.K3kControlPlane{}
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(k3kControlPlane).
		Build(r)

	if err != nil {
		return fmt.Errorf("failed setting up the K3kControlPlane controller manager: %w", err)
	}

	if err = c.Watch(
		source.Kind(mgr.GetCache(), &clusterv1beta1.Cluster{}),
		handler.EnqueueRequestsFromMapFunc(capiutil.ClusterToInfrastructureMapFunc(ctx, k3kControlPlane.GroupVersionKind(), mgr.GetClient(), &infrastructurev1.K3kCluster{})),
		predicates.ClusterUnpaused(log),
	); err != nil {
		return fmt.Errorf("failed adding a watch for ready clusters: %w", err)
	}

	if err = c.Watch(
		source.Kind(mgr.GetCache(), &infrastructurev1.K3kCluster{}),
		handler.EnqueueRequestsFromMapFunc(r.k3kClusterToK3kControlPlane(log)),
	); err != nil {
		return fmt.Errorf("failed adding a watch for K3kCluster")
	}

	return nil
}

//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=k3kcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=k3kcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=k3kcontrolplanes/finalizers,verbs=update
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=k3kclusters,verbs=get;list;watch;create;update
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=k3kclusters/status,verbs=get;list;watch;create;update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=k3k.io,resources=clusters,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,resourceNames=cluster-admin,verbs=bind
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;create
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces;serviceaccounts,verbs=get;create
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;create
//+kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=create

// Reconcile creates a K3k Upstream cluster based on the provided spec of the K3kControlPlane.
//
//nolint:gocyclo // This method can't be simple.
func (r *K3kControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if err := r.ensureUpstreamChart(ctx); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure upstream K3K release is deployed: %w", err)
	} else {
		log.Info("The upstream K3K controller Helm release is deployed without errors.")
	}
	var k3kControlPlane controlplanev1.K3kControlPlane
	err := r.Get(ctx, req.NamespacedName, &k3kControlPlane)
	if err != nil {
		if apiError.IsNotFound(err) {
			log.Error(err, "Couldn't find controlplane")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		return ctrl.Result{}, fmt.Errorf("unable to get controlplane for request %w", err)
	}
	if !k3kControlPlane.DeletionTimestamp.IsZero() {
		log.Info("About to remove upstream cluster")
		if err := r.deleteUpstreamClusters(ctx, &k3kControlPlane); err != nil {
			log.Error(err, "Couldn't remove upstream cluster")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	// we re-enqueue and take no action if we don't have a cluster owner
	capiClusterOwner, err := r.getClusterOwner(ctx, &k3kControlPlane)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get capi cluster owner: %w", err)
	}
	if capiClusterOwner == nil {
		// capi cluster owner may not be set immediately, but we don't want to process the cluster until it is
		log.Info("K3kControlPlane did not have a capi cluster owner", "controlPlane", k3kControlPlane.Name)
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Millisecond * 100,
		}, nil
	}
	if !controllerutil.ContainsFinalizer(&k3kControlPlane, finalizer) {
		controllerutil.AddFinalizer(&k3kControlPlane, finalizer)
		err := r.Update(ctx, &k3kControlPlane)
		if err != nil {
			log.Error(err, "Unable to add finalizer to k3k controlplane", "name", k3kControlPlane.Name, "namespace", k3kControlPlane.Namespace)
			return ctrl.Result{}, fmt.Errorf("unable to add finalizer")
		}
	}

	upstreamCluster, err := r.reconcileUpstreamCluster(ctx, &k3kControlPlane)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to reconcile k3k cluster: %w", err)
	}
	if upstreamCluster == nil {
		return ctrl.Result{}, fmt.Errorf("controlplane wasn't deleting, but we didn't reconcile the cluster")
	}
	log.Info("Reconciled upstream cluster for controlPlane", "upstreamCluster", upstreamCluster, "controlPlane", k3kControlPlane.Name)

	backoff := wait.Backoff{
		Steps:    5,
		Duration: 20 * time.Second,
		Factor:   2,
		Jitter:   0.1,
	}
	var config *clientcmdapi.Config
	var retryErr error
	if err := retry.OnError(backoff, apiError.IsNotFound, func() error {
		config, retryErr = r.getKubeconfig(ctx, upstreamCluster)
		if retryErr != nil {
			return retryErr
		}
		return nil
	}); err != nil {
		log.Error(err, "Hard failure when getting kubeconfig for cluster", "clusterName", upstreamCluster.Name)
		return ctrl.Result{}, fmt.Errorf("unable to get kubeconfig secret")
	}

	if err := r.createKubeconfigSecret(ctx, config, &k3kControlPlane); err != nil {
		log.Error(err, "Unable to store kubeconfig as a secret")
		return ctrl.Result{}, fmt.Errorf("unable to store kubeconfig secret")
	}
	serverURL, err := serverURLFromKubeConfig(config)
	if err != nil {
		log.Error(err, "Unable to get serverURL from kubeconfig")
		return ctrl.Result{}, fmt.Errorf("unable to resolve server url")
	}
	err = r.updateCluster(ctx, serverURL, capiClusterOwner)
	if err != nil {
		log.Error(err, "Unable to update capiCluster")
		return ctrl.Result{}, fmt.Errorf("unable to update capi cluster")
	}
	if !k3kControlPlane.Status.Ready || !k3kControlPlane.Status.Initialized {
		k3kControlPlane.Status.Ready = true
		k3kControlPlane.Status.Initialized = true
		k3kControlPlane.Status.ExternalManagedControlPlane = true
		k3kControlPlane.Status.ClusterStatus = *upstreamCluster.Status.DeepCopy()
		err = r.Status().Update(ctx, &k3kControlPlane)
		if err != nil {
			log.Error(err, "unable to update status on controlPlane")
			return ctrl.Result{}, fmt.Errorf("unable to update status")
		}
	}
	return ctrl.Result{}, nil
}

// ensureUpstreamChart tries to install a release of the upstream K3K chart or upgrade it if there is a new version.
func (r *K3kControlPlaneReconciler) ensureUpstreamChart(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)
	if err := r.Helm.ReleasePresent(ctx, r.K3KVersion); err != nil {
		values := map[string]any{}
		if errors.Is(err, driver.ErrReleaseNotFound) {
			if err := r.Helm.DeployLocalChart(ctx, values); err != nil {
				return fmt.Errorf("failed to deploy a local K3K release: %w", err)
			}
			log.Info("Successfully deployed the upstream K3K release.")
			return nil
		}
		if errors.Is(err, helm.ErrReleaseOutdated) {
			if err := r.Helm.UpgradeLocalChart(ctx, values); err != nil {
				return fmt.Errorf("failed to upgrade a local K3K release: %w", err)
			}
			log.Info("Successfully upgraded the upstream K3K release.")
			return nil
		}
		return fmt.Errorf("couldn't check for the presence of a K3K release: %w", err)
	}
	return nil
}

// getClusterOwner retrieves the CAPI cluster owning this controlPlane. Returns an error if the cluster was specified by could not be retrieved.
// can return nil, nil if no clusterOwner ref has been set yet
func (r *K3kControlPlaneReconciler) getClusterOwner(ctx context.Context, controlPlane *controlplanev1.K3kControlPlane) (*clusterv1beta1.Cluster, error) {
	var clusterKey *client.ObjectKey
	for _, ownerRef := range controlPlane.OwnerReferences {
		hasAPIVersion := ownerRef.APIVersion == clusterv1beta1.GroupVersion.String()
		if ownerRef.Name != "" && hasAPIVersion && ownerRef.Kind == clusterv1beta1.ClusterKind {
			// the owning cluster needs to be in the same namespace as the controlPlane per k8s docs
			clusterKey = &client.ObjectKey{
				Name:      ownerRef.Name,
				Namespace: controlPlane.Namespace,
			}
		}
	}
	// if we never found a cluster return a distinct signal so that the caller knows nothing has yet been set
	if clusterKey == nil {
		return nil, nil
	}
	var capiCluster clusterv1beta1.Cluster
	err := r.Get(ctx, *clusterKey, &capiCluster)
	if err != nil {
		return nil, fmt.Errorf("unable to get backing capi cluster: %w", err)
	}
	return &capiCluster, nil
}

// reconcileUpstreamCluster creates/updates the k3k cluster that the k3k controllers will recognize.
// It doesn't remove clusters if the controlPlane is being deleted.
func (r *K3kControlPlaneReconciler) reconcileUpstreamCluster(ctx context.Context, controlPlane *controlplanev1.K3kControlPlane) (*upstream.Cluster, error) {
	log := ctrl.LoggerFrom(ctx)

	clusters, err := r.getUpstreamClusters(ctx, controlPlane)
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
		if err := r.deleteUpstreamClusters(ctx, controlPlane); err != nil {
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
		err = r.Create(ctx, &upstreamCluster)
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
	err = r.Update(ctx, currentCluster)
	if err != nil {
		return nil, fmt.Errorf("unable to update cluster for controlPlane %s: %w", controlPlane.Name, err)
	}
	return currentCluster, nil
}

// deleteUpstreamClusters deletes all upstream clusters associated with this controlPlane and removes the finalizer on the controlPlane.
func (r *K3kControlPlaneReconciler) deleteUpstreamClusters(ctx context.Context, controlPlane *controlplanev1.K3kControlPlane) error {
	log := ctrl.LoggerFrom(ctx)

	clusters, err := r.getUpstreamClusters(ctx, controlPlane)
	if err != nil {
		return fmt.Errorf("unable to get current clusters for controlPlane %s/%s: %w", controlPlane.Namespace, controlPlane.Name, err)
	}
	var deleteErrs []error
	for i := range clusters {
		if err := r.Delete(ctx, &clusters[i]); err != nil && !apiError.IsNotFound(err) {
			log.Error(err, "failed to delete upstream cluster "+clusters[i].Name)
			deleteErrs = append(deleteErrs, err)
		}
	}
	if err := errors.Join(deleteErrs...); err != nil {
		return fmt.Errorf("failed to delete some upstream clusters: %w", err)
	}
	controllerutil.RemoveFinalizer(controlPlane, finalizer)
	err = r.Update(ctx, controlPlane)
	if err != nil {
		return fmt.Errorf("unable to update k3k controlplane finalizer after removing clusters: %w", err)
	}
	return nil
}

func (r *K3kControlPlaneReconciler) getUpstreamClusters(ctx context.Context, controlPlane *controlplanev1.K3kControlPlane) ([]upstream.Cluster, error) {
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
	err = r.List(ctx, &currentClusters, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("unable to list current clusters %w", err)
	}
	return currentClusters.Items, nil
}

// getKubeconfig retrieves the kubeconfig for the given upstream K3k cluster.
func (r *K3kControlPlaneReconciler) getKubeconfig(ctx context.Context, upstreamCluster *upstream.Cluster) (*clientcmdapi.Config, error) {
	// TODO: We set the expiry to one year here, but we need to continually keep this up to date
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
	rawKubeConfig, err := cfg.Extract(ctx, r.Client, upstreamCluster, u.Hostname())
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve raw kubeconfig data: %w", err)
	}
	return clientcmd.Load(rawKubeConfig)
}

// createKubeconfigSecret stores the kubeconfig into a secret which can be retrieved by CAPI later on
func (r *K3kControlPlaneReconciler) createKubeconfigSecret(ctx context.Context, kubeConfig *clientcmdapi.Config, controlPlane *controlplanev1.K3kControlPlane) error {
	ownerRef := controlPlaneOwnerRef(controlPlane)
	kubeConfigData, err := clientcmd.Write(*kubeConfig)
	if err != nil {
		return fmt.Errorf("unable to marshall kubeconfig %w", err)
	}
	secret := capiKubeconfig.GenerateSecretWithOwner(client.ObjectKeyFromObject(controlPlane), kubeConfigData, ownerRef)
	err = r.Create(ctx, secret)
	if apiError.IsAlreadyExists(err) {
		var currentSecret v1.Secret
		if err := r.Get(ctx, client.ObjectKeyFromObject(secret), &currentSecret); err != nil {
			return fmt.Errorf("detected conflict in secret %s, but was unable to get the secret: %w", secret.Name, err)
		}
		// TODO: We probably should correct the capi label here as well
		currentSecret = *currentSecret.DeepCopy()
		currentSecret.Data = secret.Data
		if err := r.Update(ctx, &currentSecret); err != nil {
			return fmt.Errorf("unable to update secret %s: %w", secret.Name, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("unable to create secret %s with non-conflict error: %w", secret.Name, err)
	}
	return nil
}

func (r *K3kControlPlaneReconciler) updateCluster(ctx context.Context, serverURL *url.URL, capiCluster *clusterv1beta1.Cluster) error {
	clusterRef := capiCluster.Spec.InfrastructureRef
	groupVersion := infrastructurev1.GroupVersion
	if clusterRef == nil || clusterRef.APIVersion != groupVersion.String() && clusterRef.Kind != "K3kCluster" {
		return fmt.Errorf("unable to update cluster, infrastructure is not for a k3k cluster %v", clusterRef)
	}
	k3kObjectKey := client.ObjectKey{
		Name:      clusterRef.Name,
		Namespace: capiCluster.Namespace,
	}
	var k3kCluster infrastructurev1.K3kCluster
	if err := r.Get(ctx, k3kObjectKey, &k3kCluster); err != nil {
		return fmt.Errorf("unable to get k3k cluster %s/%s: %w", k3kObjectKey.Namespace, k3kObjectKey.Name, err)
	}
	k3kCluster = *k3kCluster.DeepCopy()
	host := serverURL.Hostname()
	k3kCluster.Spec.ControlPlaneEndpoint.Host = host
	port := serverURL.Port()
	if port != "" {
		intPort, err := strconv.ParseInt(port, 10, 32)
		if err != nil {
			return fmt.Errorf("unable to cast port %s from string to int: %w", port, err)
		}
		k3kCluster.Spec.ControlPlaneEndpoint.Port = int32(intPort)
	}
	if err := r.Update(ctx, &k3kCluster); err != nil {
		return fmt.Errorf("unable to update k3kCluster %s with host and port: %w", k3kCluster.Name, err)
	}
	k3kCluster.Status = infrastructurev1.K3kClusterStatus{}
	k3kCluster.Status.Ready = true
	if err := r.Status().Update(ctx, &k3kCluster); err != nil {
		return fmt.Errorf("unable to update k3kCluster %s status: %w", k3kCluster.Name, err)
	}
	return nil
}

func serverURLFromKubeConfig(config *clientcmdapi.Config) (*url.URL, error) {
	ctx, ok := config.Contexts[config.CurrentContext]
	if !ok {
		return nil, fmt.Errorf("current context %s was not a valid context", config.CurrentContext)
	}
	cluster, ok := config.Clusters[ctx.Cluster]
	if !ok {
		return nil, fmt.Errorf("cluster %s defined in context %s was not a valid cluster", ctx.Cluster, config.CurrentContext)
	}
	serverURL, err := url.Parse(cluster.Server)
	if err != nil {
		return nil, fmt.Errorf("unable to parse server url %s into a url", serverURL)
	}
	if serverURL == nil {
		return nil, fmt.Errorf("serverURL was parsed, but was nil")
	}
	return serverURL, nil
}

func controlPlaneOwnerRef(controlPlane *controlplanev1.K3kControlPlane) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: controlPlane.APIVersion,
		Kind:       controlPlane.Kind,
		Name:       controlPlane.Name,
		UID:        controlPlane.UID,
	}
}

func (r *K3kControlPlaneReconciler) k3kClusterToK3kControlPlane(log logr.Logger) handler.MapFunc {
	return func(ctx context.Context, o client.Object) []ctrl.Request {
		k3kCluster, ok := o.(*infrastructurev1.K3kCluster)
		if !ok {
			log.Error(fmt.Errorf("expected a K3kCluster but got a %T", o), "Expected K3kCluster")
			return nil
		}

		if !k3kCluster.ObjectMeta.DeletionTimestamp.IsZero() {
			return nil
		}

		cluster, err := capiutil.GetOwnerCluster(ctx, r.Client, k3kCluster.ObjectMeta)
		if err != nil {
			log.Error(err, "failed to get owning cluster")
			return nil
		}
		if cluster == nil {
			return nil
		}

		controlPlaneRef := cluster.Spec.ControlPlaneRef
		if controlPlaneRef == nil || controlPlaneRef.Kind != k3kControlPlaneKind {
			return nil
		}

		return []ctrl.Request{
			{
				NamespacedName: types.NamespacedName{
					Name:      controlPlaneRef.Name,
					Namespace: controlPlaneRef.Namespace,
				},
			},
		}
	}
}
