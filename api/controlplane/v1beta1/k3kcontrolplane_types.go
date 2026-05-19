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

package v1beta1

import (
	upstream "github.com/rancher/k3k/pkg/apis/k3k.io/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// K3kControlPlaneSpec defines the desired state of K3kControlPlane
type K3kControlPlaneSpec struct {
	// HostKubeconfig specifies the location of the Kubernetes secret containing the kubeconfig for the host cluster where the K3k cluster will be installed.
	// If not provided, the K3k cluster will be created in the current context's cluster.
	//
	// +optional
	HostKubeconfig *HostKubeconfigLocation `json:"hostKubeconfig,omitempty"`

	// HostTargetNamespace is the host namespace where the virtual cluster will be installed.
	// If not provided a namespace with the k3k-<cluster_name> prefix will be created.
	//
	// +optional
	HostTargetNamespace string `json:"hostTargetNamespace,omitempty"`

	// ClusterSpec contains the k3k cluster configuration, embedded inline from upstream.
	// All fields from github.com/rancher/k3k/pkg/apis/k3k.io/v1beta1.ClusterSpec are available here.
	upstream.ClusterSpec `json:",inline"`
}

// HostKubeconfigLocation specifies a source of a kubeconfig used to access a cluster.
type HostKubeconfigLocation struct {
	// SecretName is the name of the secret containing the kubconfig data.
	SecretName string `json:"secretName"`
	// SecretNamespace is the namespace of the secret containing the kubeconfig data.
	SecretNamespace string `json:"secretNamespace"`
}

// K3kControlPlaneStatus defines the observed state of K3kControlPlane
type K3kControlPlaneStatus struct {
	// Initialized conforms to the upstream CAPI interface - true when the controlplane is contactable.
	Initialized bool `json:"initialized"`
	// Ready conforms to the upstream CAPI interface - true when the target API Server is ready to receive requests.
	Ready bool `json:"ready"`
	// ExternalManagedControlPlane conforms to the upstream CAPI interface - true if node objects do not exist in the controlplane.
	// While K3k does have specific pods for the controlplane, these don't correspond to specific CAPI objects/machines, so in the
	// current state this will always be true.
	ExternalManagedControlPlane bool `json:"externalManagedControlPlane"`
	// ClusterStatus is the status of the backing K3k cluster type.
	ClusterStatus upstream.ClusterStatus `json:"clusterStatus"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// K3kControlPlane is the Schema for the K3kcontrolplanes API
type K3kControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired spec of the K3k controlPlane.
	Spec K3kControlPlaneSpec `json:"spec,omitempty"`
	// Status is the current status of the K3k controlPlane.
	Status K3kControlPlaneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// K3kControlPlaneList contains a list of K3kControlPlane
type K3kControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K3kControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&K3kControlPlane{}, &K3kControlPlaneList{})
}
