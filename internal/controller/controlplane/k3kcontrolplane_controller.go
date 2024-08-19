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
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	upstream "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apiError "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/retry"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capiutil "sigs.k8s.io/cluster-api/util"
	capiKubeconfig "sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-k3k/api/controlplane/v1alpha1"
	infrastructurev1 "github.com/rancher-sandbox/cluster-api-provider-k3k/api/infrastructure/v1alpha1"
	"github.com/rancher-sandbox/cluster-api-provider-k3k/internal/helm"
)

const (
	ownerNameLabel      = "app.cattle.io/owner-name"
	ownerNamespaceLabel = "app.cattle.io/owner-ns"
	finalizer           = "app.cattle.io/upstream-remove"
)

type scope struct {
	logr.Logger

	// k3kControlPlane is the controlPlane that this particular reconcile action was initiated by
	k3kControlPlane *controlplanev1.K3kControlPlane
	// cluster is the cluster which owns the k3kControlPlane
	cluster *clusterv1beta1.Cluster
	// hostClient is the client for the cluster which will host the targeted k3k cluster
	hostClient client.Client
	// hostHelmClient is the helm client for the cluster which will host the targeted k3k cluster
	hostHelmClient helm.Client
}

// K3kControlPlaneReconciler reconciles a K3kControlPlane object
type K3kControlPlaneReconciler struct {
	client.Client
	// K3KVersion is the version of the upstream k3k chart to deploy/upgrade to in the cluster
	K3KVersion string
	// defaultConfig is the kubeconfig from the current cluster. Used when creating a helm Client for the current
	// cluster
	defaultConfig *rest.Config
}

// SetupWithManager sets up the controller with the Manager.
func (r *K3kControlPlaneReconciler) SetupWithManager(_ context.Context, mgr ctrl.Manager) error {
	r.defaultConfig = mgr.GetConfig()
	return ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1.K3kControlPlane{}).
		Complete(r)
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
func (r *K3kControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	k3kControlPlane := &controlplanev1.K3kControlPlane{}
	err := r.Get(ctx, req.NamespacedName, k3kControlPlane)
	if err != nil {
		if apiError.IsNotFound(err) {
			log.Error(err, "Couldn't find controlplane")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		return ctrl.Result{}, fmt.Errorf("unable to get controlplane for request %w", err)
	}

	cluster, err := capiutil.GetOwnerCluster(ctx, r, k3kControlPlane.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get capi cluster owner: %w", err)
	}
	if cluster == nil {
		// capi cluster owner may not be set immediately, but we don't want to process the cluster until it is
		log.Info("K3kControlPlane did not have a capi cluster owner", "controlPlane", k3kControlPlane.Name)
		return ctrl.Result{}, nil
	}
	hostClient, hostHelmClient, err := r.getHostClients(ctx, *k3kControlPlane)
	if err != nil {
		log.Info("Unable to get host-kubeconfig for controlplane", "controlPlane", k3kControlPlane.Name, "error", err)
	}

	scope := &scope{
		Logger:          log,
		k3kControlPlane: k3kControlPlane,
		cluster:         cluster,
		hostClient:      hostClient,
		hostHelmClient:  *hostHelmClient,
	}

	if !k3kControlPlane.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, scope)
	}

	return r.reconcileNormal(ctx, scope)
}

func (r *K3kControlPlaneReconciler) reconcileNormal(ctx context.Context, scope *scope) (ctrl.Result, error) {
	if err := ensureUpstreamChart(ctx, scope, r.K3KVersion); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure upstream K3K release is deployed: %w", err)
	} else {
		scope.Info("The upstream K3K controller Helm release is deployed without errors.")
	}
	if !controllerutil.ContainsFinalizer(scope.k3kControlPlane, finalizer) {
		controllerutil.AddFinalizer(scope.k3kControlPlane, finalizer)
		err := r.Update(ctx, scope.k3kControlPlane)
		if err != nil {
			scope.Error(err, "Unable to add finalizer to k3k controlplane", "name", scope.k3kControlPlane.Name, "namespace", scope.k3kControlPlane.Namespace)
			return ctrl.Result{}, fmt.Errorf("unable to add finalizer")
		}
	}

	upstreamCluster, err := reconcileUpstreamCluster(ctx, scope)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to reconcile k3k cluster: %w", err)
	}
	if upstreamCluster == nil {
		return ctrl.Result{}, fmt.Errorf("controlplane wasn't deleting, but we didn't reconcile the cluster")
	}
	scope.Info("Reconciled upstream cluster for controlPlane", "upstreamCluster", upstreamCluster, "controlPlane", scope.k3kControlPlane.Name)

	backoff := wait.Backoff{
		Steps:    5,
		Duration: 20 * time.Second,
		Factor:   2,
		Jitter:   0.1,
	}
	var config *clientcmdapi.Config
	var retryErr error
	if err := retry.OnError(backoff, apiError.IsNotFound, func() error {
		config, retryErr = getKubeconfig(ctx, scope, upstreamCluster)
		if retryErr != nil {
			return retryErr
		}
		return nil
	}); err != nil {
		scope.Error(err, "Hard failure when getting kubeconfig for cluster", "clusterName", upstreamCluster.Name)
		return ctrl.Result{}, fmt.Errorf("unable to get kubeconfig secret")
	}

	if err := r.createKubeconfigSecret(ctx, config, scope.k3kControlPlane); err != nil {
		scope.Error(err, "Unable to store kubeconfig as a secret")
		return ctrl.Result{}, fmt.Errorf("unable to store kubeconfig secret")
	}
	serverURL, err := serverURLFromKubeConfig(config)
	if err != nil {
		scope.Error(err, "Unable to get serverURL from kubeconfig")
		return ctrl.Result{}, fmt.Errorf("unable to resolve server url")
	}
	err = r.updateCluster(ctx, serverURL, scope.cluster)
	if err != nil {
		scope.Error(err, "Unable to update capiCluster")
		return ctrl.Result{}, fmt.Errorf("unable to update capi cluster")
	}
	if !scope.k3kControlPlane.Status.Ready || scope.k3kControlPlane.Status.Initialized {
		scope.k3kControlPlane.Status.Ready = true
		scope.k3kControlPlane.Status.Initialized = true
		scope.k3kControlPlane.Status.ExternalManagedControlPlane = true
		scope.k3kControlPlane.Status.ClusterStatus = *upstreamCluster.Status.DeepCopy()
		err = r.Status().Update(ctx, scope.k3kControlPlane)
		if err != nil {
			scope.Error(err, "unable to update status on controlPlane")
			return ctrl.Result{}, fmt.Errorf("unable to update status")
		}
	}
	return ctrl.Result{}, nil
}

// reconcileDelete handles deleting the resources created for this controlplane previously
func (r *K3kControlPlaneReconciler) reconcileDelete(ctx context.Context, scope *scope) (ctrl.Result, error) {
	scope.Info("About to remove upstream cluster")
	controlPlane := scope.k3kControlPlane
	if err := deleteUpstreamClusters(ctx, scope); err != nil {
		scope.Error(err, "Couldn't remove upstream cluster")
		return ctrl.Result{}, err
	}
	controllerutil.RemoveFinalizer(controlPlane, finalizer)
	err := r.Update(ctx, controlPlane)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to update k3k controlplane finalizer after removing clusters: %w", err)
	}
	return ctrl.Result{}, nil
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

func (r *K3kControlPlaneReconciler) getHostClients(ctx context.Context, controlPlane controlplanev1.K3kControlPlane) (client.Client, *helm.Client, error) {
	if controlPlane.Spec.HostKubeconfig == nil {
		// if the HostKubeconfig isn't set, then the current cluster is the same as the host cluster, so use the client we already have
		helmClient, err := helmClientForConfig(r.defaultConfig, r.Client)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to contruct clients from default in-cluster config %w", err)
		}
		return r.Client, helmClient, nil
	}
	var kubeconfigSecret v1.Secret
	secretKey := client.ObjectKey{
		Name:      controlPlane.Spec.HostKubeconfig.SecretName,
		Namespace: controlPlane.Spec.HostKubeconfig.SecretNamespace,
	}
	err := r.Get(ctx, secretKey, &kubeconfigSecret)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to get specified secret %s/%s: %w", secretKey.Namespace, secretKey.Name, err)
	}
	kubeconfigBytes, ok := kubeconfigSecret.Data[secret.KubeconfigDataName]
	if !ok {
		return nil, nil, fmt.Errorf("secret %s/%s found but no kubeconfig found at key %s", secretKey.Namespace, secretKey.Name, secret.KubeconfigDataName)
	}
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to load secret content in kubeconfig %s/%s as a kubeconfig: %w", secretKey.Namespace, secretKey.Name, err)
	}
	scheme := runtime.NewScheme()
	err = clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to add basic client-go types to scheme for kubeconfig %s/%s: %w", secretKey.Namespace, secretKey.Name, err)
	}
	err = upstream.AddToScheme(scheme)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to add upstream k3k types to scheme for kubeconfig %s/%s: %w", secretKey.Namespace, secretKey.Name, err)
	}
	// note: providing no cache options disables the cache. This is ok since we don't want to read in all objects, but may need to chagne in the future.
	hostClient, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create client from kubeconfig %s/%s: %w", secretKey.Namespace, secretKey.Name, err)
	}
	helmClient, err := helmClientForConfig(config, hostClient)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create helm client from kubeconfig %s/%s: %w", secretKey.Namespace, secretKey.Name, err)
	}
	return hostClient, helmClient, nil
}

func helmClientForConfig(config *rest.Config, client client.Client) (*helm.Client, error) {
	k8s, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get client set: %w", err)
	}
	restMapper := client.RESTMapper()
	cache := memory.NewMemCacheClient(k8s.Discovery())
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	getter := helm.SimpleRESTClientGetter{
		ClientConfig:    kubeConfig,
		RESTConfig:      config,
		CachedDiscovery: cache,
		RESTMapper:      restMapper,
	}
	helmClient := helm.New(&getter, "charts/k3k", "k3k-operator", "k3k-internal")
	return &helmClient, nil
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
