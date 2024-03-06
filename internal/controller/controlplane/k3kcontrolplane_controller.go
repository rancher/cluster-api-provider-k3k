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
	"reflect"
	"strconv"
	"strings"
	"time"

	upstream "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	"github.com/rancher/k3k/pkg/controller/kubeconfig"
	"github.com/rancher/k3k/pkg/controller/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	capiKubeconfig "sigs.k8s.io/cluster-api/util/kubeconfig"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	controlplane "github.com/mbolotsuse/cluster-api-provider-k3k/api/controlplane/v1beta1"
	infrastructurev1 "github.com/mbolotsuse/cluster-api-provider-k3k/api/infrastructure/v1beta1"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	ownerLabel = "app.cattle.io/owner"
)

// K3kControlPlaneReconciler reconciles a K3kControlPlane object
type K3kControlPlaneReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=k3kcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=k3kcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=k3kcontrolplanes/finalizers,verbs=update
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=k3kclusters,verbs=get;list;watch;create;update
//+kubebuilder:rbac:groups=cluster.cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update

// Reconcile creates a K3k Upstream cluster based on the provided spec of the K3kControlPlane.
func (r *K3kControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	var k3kControlPlane controlplane.K3kControlPlane
	err := r.Get(ctx, req.NamespacedName, &k3kControlPlane)
	ctrl.Log.Info("got request", "object", k3kControlPlane, "err", err)
	// we re-enqueue and take no action if we don't have a cluster owner
	capiClusterOwner, err := r.getClusterOwner(ctx, k3kControlPlane)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get capi cluster owner: %w", err)
	}
	if capiClusterOwner == nil {
		// capi cluster owner may not be set immediately, but we don't want to process the cluster until it is
		ctrl.Log.Info("K3kControlPlane did not have a capi cluster owner", "controlPlane", k3kControlPlane.Name)
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Millisecond * 100,
		}, nil
	}

	upstreamCluster, err := r.reconcileUpstreamCluster(ctx, k3kControlPlane)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to reconcile k3k cluster: %w", err)
	}
	ctrl.Log.Info("Reconciled upstream cluster for controlPlane", "upstreamCluster", upstreamCluster, "controlPlane", k3kControlPlane.Name)
	kubeconfig, err := r.getKubeconfig(ctx, *upstreamCluster)
	if errors.IsNotFound(err) || kubeconfig == nil {
		// TODO: Right now we re-enqueue the whole object to wait for the kubeconfig. This is probably quite inefficient, and stops us
		// from breaking after a long time has passed
		ctrl.Log.Info("Kubeconfig secret not found yet for cluster, re-enqueueing", "clusterName", upstreamCluster.Name, "secretError", err)
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Second * 5,
		}, nil
	} else if err != nil {
		ctrl.Log.Error(err, "Hard failure when getting kubeconfig for cluster", "clusterName", upstreamCluster.Name)
		return ctrl.Result{}, fmt.Errorf("unable to get kubeconfig secret")
	}
	err = r.createKubeconfigSecret(ctx, *kubeconfig, k3kControlPlane)
	if err != nil {
		ctrl.Log.Error(err, "Unable to store kubeconfig as a secret")
		return ctrl.Result{}, fmt.Errorf("unable to store kubeconfig secret")
	}
	serverUrl, err := serverUrlFromKubeConfig(*kubeconfig)
	if err != nil {
		ctrl.Log.Error(err, "Unable to get serverUrl from kubeconfig")
		return ctrl.Result{}, fmt.Errorf("unable to resolve server url")
	}
	err = r.updateCluster(ctx, serverUrl, capiClusterOwner)
	if err != nil {
		ctrl.Log.Error(err, "Unable to update capiCluster")
		return ctrl.Result{}, fmt.Errorf("unable to update capi cluster")
	}
	if !k3kControlPlane.Status.Ready || !k3kControlPlane.Status.Initialized {
		k3kControlPlane.Status.Ready = true
		k3kControlPlane.Status.Initialized = true
		k3kControlPlane.Status.ExternalManagedControlPlane = true
		k3kControlPlane.Status.ClusterStatus = *upstreamCluster.Status.DeepCopy()
		err = r.Status().Update(ctx, &k3kControlPlane)
		if err != nil {
			ctrl.Log.Error(err, "unable to update status on controlPlane")
			return ctrl.Result{}, fmt.Errorf("unable to update status")
		}
	}
	return ctrl.Result{}, nil
}

// getClusterOwner retrieves the CAPI cluster owning this controlPlane. Returns an error if the cluster was specified by could not be retrieved.
// can return nil, nil if no clusterOwner ref has been set yet
func (r *K3kControlPlaneReconciler) getClusterOwner(ctx context.Context, controlPlane controlplane.K3kControlPlane) (*clusterv1beta1.Cluster, error) {
	var clusterKey *client.ObjectKey
	for _, ownerRef := range controlPlane.OwnerReferences {
		hasAPIVersion := ownerRef.APIVersion == clusterv1beta1.GroupVersion.Version || ownerRef.APIVersion == "v1alpha4"
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

// reconcileUpstreamCluster creates/updates the k3k cluster that the k3k controllers will recognize. Owner refs handle deletion.
func (r *K3kControlPlaneReconciler) reconcileUpstreamCluster(ctx context.Context, controlPlane controlplane.K3kControlPlane) (*upstream.Cluster, error) {
	var currentClusters upstream.ClusterList
	ownerRequirement, err := labels.NewRequirement(ownerLabel, selection.Equals, []string{controlPlane.Name})
	if err != nil {
		return nil, fmt.Errorf("unable to form label for controlPlane %s: %w", controlPlane.Name, err)
	}
	err = r.List(ctx, &currentClusters, &client.ListOptions{LabelSelector: labels.NewSelector().Add(*ownerRequirement)})
	if err != nil {
		return nil, fmt.Errorf("unable to list current clusters %w", err)
	}
	if len(currentClusters.Items) > 1 {
		// TODO: at this point we might just want to delete all of of the current clusters and re-provision.
		var names []string
		for _, cluster := range currentClusters.Items {
			names = append(names, cluster.Name)
		}
		joinedNames := strings.Join(names, ", ")
		return nil, fmt.Errorf("controlplane %s is owned by two or more clusters: %s, refusing to process further", controlPlane.Name, joinedNames)
	}
	if len(currentClusters.Items) == 0 {
		upstreamCluster := upstream.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: controlPlane.Name + "-",
				Labels: map[string]string{
					ownerLabel: controlPlane.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					controlPlaneOwnerRef(controlPlane),
				},
			},
			Spec: controlPlane.Spec.ClusterSpec,
		}
		err = r.Create(ctx, &upstreamCluster)
		if err != nil {
			return nil, fmt.Errorf("unable to create cluster for controlPlane %s: %w", controlPlane.Name, err)
		}
	}
	// at this point we have exactly one cluster
	currentCluster := currentClusters.Items[0].DeepCopy()
	if reflect.DeepEqual(currentCluster.Spec, controlPlane.Spec.ClusterSpec) {
		return currentCluster, nil
	}
	currentCluster.Spec = controlPlane.Spec.ClusterSpec
	err = r.Update(ctx, currentCluster)
	if err != nil {
		return nil, fmt.Errorf("unable to update cluster for controlPlane %s: %w", controlPlane.Name, err)
	}
	return currentCluster, nil
}

// getKubeconfig retrieves the kubeconfig for the given cluster
func (r *K3kControlPlaneReconciler) getKubeconfig(ctx context.Context, upstreamCluster upstream.Cluster) (*clientcmdapi.Config, error) {
	// TODO: We set the expiry to one year here, but we need to continually keep this up to date
	cfg := kubeconfig.KubeConfig{
		CN:         util.AdminCommonName,
		ExpiryDate: time.Hour * 24 * 365,
	}
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get current config to derive hostname: %w", err)
	}
	url, err := url.Parse(restConfig.Host)
	if err != nil {
		return nil, fmt.Errorf("unable to extract url from specificed hostname: %s, %w", restConfig.Host, err)
	}
	rawKubeConfig, err := cfg.Extract(ctx, r.Client, &upstreamCluster, url.Hostname())
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve raw kubeconfig data: %w", err)
	}
	return clientcmd.Load(rawKubeConfig)
}

// createKubeconfigSecret stores the kubeconfig into a secret which can be retrieved by CAPI later on
func (r *K3kControlPlaneReconciler) createKubeconfigSecret(ctx context.Context, kubeConfig clientcmdapi.Config, controlPlane controlplane.K3kControlPlane) error {
	ownerRef := controlPlaneOwnerRef(controlPlane)
	kubeConfigData, err := clientcmd.Write(kubeConfig)
	if err != nil {
		return fmt.Errorf("unable to marshall kubeconfig %w", err)
	}
	secret := capiKubeconfig.GenerateSecretWithOwner(client.ObjectKeyFromObject(&controlPlane), kubeConfigData, ownerRef)
	err = r.Create(ctx, secret)
	if errors.IsConflict(err) {
		var currentSecret v1.Secret
		err = r.Get(ctx, client.ObjectKeyFromObject(secret), &currentSecret)
		if err != nil {
			return fmt.Errorf("detected conflict in secret %s, but was unable to get the secret: %w", secret.Name, err)
		}
		// TODO: We probably should correct the capi label here as well
		currentSecret = *currentSecret.DeepCopy()
		currentSecret.Data = secret.Data
		err = r.Update(ctx, &currentSecret)
		if err != nil {
			return fmt.Errorf("unable to update secret %s: %w", secret.Name, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("unable to create secret %s with non-conflict error: %w", secret.Name, err)
	}
	return nil
}

func (r *K3kControlPlaneReconciler) updateCluster(ctx context.Context, serverUrl url.URL, capiCluster *clusterv1beta1.Cluster) error {
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
	err := r.Get(ctx, k3kObjectKey, &k3kCluster)
	if err != nil {
		return fmt.Errorf("unable to get k3k cluster %s/%s: %w", k3kObjectKey.Namespace, k3kObjectKey.Name, err)
	}
	k3kCluster = *k3kCluster.DeepCopy()
	host := serverUrl.Hostname()
	k3kCluster.Spec.ControlPlaneEndpoint.Host = host
	port := serverUrl.Port()
	if port != "" {
		intPort, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("unable to cast port %s from string to int: %w", port, err)
		}
		k3kCluster.Spec.ControlPlaneEndpoint.Port = int32(intPort)
	}
	err = r.Update(ctx, &k3kCluster)
	if err != nil {
		return fmt.Errorf("unable to update k3kCluster %s with host and port: %w", k3kCluster.Name, err)
	}
	return nil
}

func serverUrlFromKubeConfig(kubeconfig clientcmdapi.Config) (url.URL, error) {
	context, ok := kubeconfig.Contexts[kubeconfig.CurrentContext]
	if !ok {
		return url.URL{}, fmt.Errorf("current context %s was not a valid context", kubeconfig.CurrentContext)
	}
	cluster, ok := kubeconfig.Clusters[context.Cluster]
	if !ok {
		return url.URL{}, fmt.Errorf("cluster %s defined in context %s was not a valid cluster", context.Cluster, kubeconfig.CurrentContext)
	}
	serverUrl, err := url.Parse(cluster.Server)
	if err != nil {
		return url.URL{}, fmt.Errorf("unable to parse server url %s into a url", serverUrl)
	}
	if serverUrl == nil {
		return url.URL{}, fmt.Errorf("serverUrl was parsed, but was nil")
	}
	return *serverUrl, nil
}

func controlPlaneOwnerRef(controlPlane controlplane.K3kControlPlane) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: controlPlane.APIVersion,
		Kind:       controlPlane.Kind,
		Name:       controlPlane.Name,
		UID:        controlPlane.UID,
	}

}

// SetupWithManager sets up the controller with the Manager.
func (r *K3kControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&controlplane.K3kControlPlane{}).
		Complete(r)
}
