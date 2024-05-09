package controlplane

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	infrastructurev1 "github.com/rancher-sandbox/cluster-api-provider-k3k/api/infrastructure/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var sentinelEmptyClient = fake.NewClientBuilder().WithObjects().Build()

func TestK3kClusterNotDeleting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expected bool
		event    event.UpdateEvent
	}{
		{
			name:     "not a k3k cluster",
			expected: false,
			event: event.UpdateEvent{
				ObjectNew: &unstructured.Unstructured{},
			},
		},
		{
			name:     "deleting k3k cluster",
			expected: false,
			event: event.UpdateEvent{
				ObjectNew: &infrastructurev1.K3kCluster{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
				},
			},
		},
		{
			name:     "non deleting k3k cluster",
			expected: true,
			event: event.UpdateEvent{
				ObjectNew: &infrastructurev1.K3kCluster{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := k3kClusterNotDeleting(logr.Discard()).Update(tt.event)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestK3kClusterAdoptedByCAPI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expected bool
		event    event.UpdateEvent
	}{
		{
			name:     "not a k3k cluster",
			expected: false,
			event: event.UpdateEvent{
				ObjectNew: &unstructured.Unstructured{},
			},
		},
		{
			name:     "no owner references k3k cluster",
			expected: false,
			event: event.UpdateEvent{
				ObjectNew: &infrastructurev1.K3kCluster{},
			},
		},
		{
			name:     "wrong kind in owner reference",
			expected: false,
			event: event.UpdateEvent{
				ObjectNew: &infrastructurev1.K3kCluster{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "NotACluster",
							},
						},
					},
				},
			},
		},
		{
			name:     "wrong apiversion in owner reference",
			expected: false,
			event: event.UpdateEvent{
				ObjectNew: &infrastructurev1.K3kCluster{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "Cluster",
								APIVersion: "notcluster.x-k8s.io/v1beta1",
							},
						},
					},
				},
			},
		},
		{
			name:     "capi cluster owner reference",
			expected: true,
			event: event.UpdateEvent{
				ObjectNew: &infrastructurev1.K3kCluster{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "Cluster",
								APIVersion: "cluster.x-k8s.io/v1beta1",
							},
						},
					},
				},
			},
		},
		{
			name:     "capi cluster owner reference old apiversion",
			expected: true,
			event: event.UpdateEvent{
				ObjectNew: &infrastructurev1.K3kCluster{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "Cluster",
								APIVersion: "cluster.x-k8s.io/v1alpha4",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := k3kClusterAdoptedByCAPI(logr.Discard()).Update(tt.event)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestK3kClusterToK3kControlPlane(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expected []ctrl.Request
		cluster  *clusterv1beta1.Cluster
		object   client.Object
	}{
		{
			name:     "not a k3k cluster",
			expected: nil,
			object:   nil,
		},
		{
			name:     "no capi cluster owner",
			expected: nil,
			object:   &infrastructurev1.K3kCluster{},
		},
		{
			name:     "capi cluster does not exist",
			expected: nil,
			object: &infrastructurev1.K3kCluster{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "cluster.x-k8s.io/v1beta1",
							Kind:       "Cluster",
							Name:       "testcluster",
							Controller: ptr.To(true),
						},
					},
				},
			},
		},
		{
			name:     "capi cluster is paused",
			expected: nil,
			cluster: &clusterv1beta1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cluster.x-k8s.io/v1beta1",
					Kind:       "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testcluster",
				},
				Spec: clusterv1beta1.ClusterSpec{
					Paused: true,
				},
			},
			object: &infrastructurev1.K3kCluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testk3kcluster",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "cluster.x-k8s.io/v1beta1",
							Kind:       "Cluster",
							Name:       "testcluster",
							Controller: ptr.To(true),
						},
					},
				},
			},
		},
		{
			name:     "cluster has no controlplane ref",
			expected: nil,
			cluster: &clusterv1beta1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cluster.x-k8s.io/v1beta1",
					Kind:       "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testcluster",
				},
			},
			object: &infrastructurev1.K3kCluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testk3kcluster",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "cluster.x-k8s.io/v1beta1",
							Kind:       "Cluster",
							Name:       "testcluster",
							Controller: ptr.To(true),
						},
					},
				},
			},
		},
		{
			name:     "cluster has controlplane ref of different kind",
			expected: nil,
			cluster: &clusterv1beta1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cluster.x-k8s.io/v1beta1",
					Kind:       "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testcluster",
				},
				Spec: clusterv1beta1.ClusterSpec{
					ControlPlaneRef: &v1.ObjectReference{
						Kind: "NotaK3KControlPlane",
					},
				},
			},
			object: &infrastructurev1.K3kCluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testk3kcluster",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "cluster.x-k8s.io/v1beta1",
							Kind:       "Cluster",
							Name:       "testcluster",
							Controller: ptr.To(true),
						},
					},
				},
			},
		},
		{
			name: "valid k3k cluster and controlplane",
			expected: []reconcile.Request{
				{
					types.NamespacedName{
						Namespace: "default",
						Name:      "testk3kcontrolplane",
					},
				},
			},
			cluster: &clusterv1beta1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testcluster",
				},
				Spec: clusterv1beta1.ClusterSpec{
					ControlPlaneRef: &v1.ObjectReference{
						Kind:      "K3kControlPlane",
						Namespace: "default",
						Name:      "testk3kcontrolplane",
					},
				},
			},
			object: &infrastructurev1.K3kCluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testk3kcluster",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "cluster.x-k8s.io/v1beta1",
							Kind:       "Cluster",
							Name:       "testcluster",
							Controller: ptr.To(true),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r := &K3kControlPlaneReconciler{
				Client: sentinelEmptyClient,
			}
			if tt.cluster != nil {
				scheme := runtime.NewScheme()
				_ = clusterv1beta1.AddToScheme(scheme)
				r.Client = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(tt.cluster.DeepCopy()).
					Build()
			}

			result := r.k3kClusterToK3kControlPlane(logr.Discard())(context.Background(), tt.object)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestClusterToK3kControlPlane(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expected []ctrl.Request
		object   client.Object
	}{
		{
			name:     "not a capi cluster",
			expected: nil,
			object:   nil,
		},
		{
			name:     "capi cluster is paused",
			expected: nil,
			object: &clusterv1beta1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cluster.x-k8s.io/v1beta1",
					Kind:       "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testcluster",
				},
				Spec: clusterv1beta1.ClusterSpec{
					Paused: true,
				},
			},
		},
		{
			name:     "cluster has no controlplane ref",
			expected: nil,
			object: &clusterv1beta1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cluster.x-k8s.io/v1beta1",
					Kind:       "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testcluster",
				},
			},
		},
		{
			name:     "cluster has controlplane ref of different kind",
			expected: nil,
			object: &clusterv1beta1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cluster.x-k8s.io/v1beta1",
					Kind:       "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testcluster",
				},
				Spec: clusterv1beta1.ClusterSpec{
					ControlPlaneRef: &v1.ObjectReference{
						Kind: "NotaK3KControlPlane",
					},
				},
			},
		},
		{
			name: "valid cluster",
			expected: []reconcile.Request{
				{
					types.NamespacedName{
						Namespace: "default",
						Name:      "testk3kcontrolplane",
					},
				},
			},
			object: &clusterv1beta1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "testcluster",
				},
				Spec: clusterv1beta1.ClusterSpec{
					ControlPlaneRef: &v1.ObjectReference{
						Kind:      "K3kControlPlane",
						Namespace: "default",
						Name:      "testk3kcontrolplane",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r := &K3kControlPlaneReconciler{
				Client: sentinelEmptyClient,
			}
			if tt.object != nil {
				scheme := runtime.NewScheme()
				_ = clusterv1beta1.AddToScheme(scheme)
				r.Client = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(tt.object.(*clusterv1beta1.Cluster).DeepCopy()).
					Build()
			}

			result := r.ClusterToK3kControlPlane(logr.Discard())(context.Background(), tt.object)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}
