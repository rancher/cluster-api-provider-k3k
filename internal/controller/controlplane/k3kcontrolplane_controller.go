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
	"reflect"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	controlplane "github.com/mbolotsuse/cluster-api-provider-k3k/api/controlplane/v1beta1"
	upstream "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
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
	ctrl.Log.Info("Created upstream cluster for controlPlane", "upstreamCluster", upstreamCluster, "controlPlane", k3kControlPlane.Name)
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
					{
						APIVersion: controlPlane.APIVersion,
						Kind:       controlPlane.Kind,
						Name:       controlPlane.Name,
						UID:        controlPlane.UID,
					},
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

// SetupWithManager sets up the controller with the Manager.
func (r *K3kControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&controlplane.K3kControlPlane{}).
		Complete(r)
}
