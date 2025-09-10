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

package v1alpha1

import (
	upstream "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// K3kControlPlaneSpec defines the desired state of K3kControlPlane
type K3kControlPlaneSpec struct {
	// HostKubeconfig is the location of the kubeconfig to the host cluster that the K3k cluster should install in.
	// If not supplied the K3k cluster will be made in the current cluster.
	//
	// +optional
	HostKubeconfig *HostKubeconfigLocation `json:"hostKubeconfig,omitempty"`

	// HostTargetNamespace is the host namespace where the virtual cluster will be installed to.
	// If not provided a namespace with the k3k-<cluster_name> prefix will be created.
	//
	// +optional
	HostTargetNamespace string `json:"hostTargetNamespace,omitempty"`

	// The following fields are copied from the github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1.ClusterSpec

	// Servers is the number of K3s pods to run in server (controlplane) mode.
	Servers *int32 `json:"servers,omitempty"`
	// Agents is the number of K3s pods to run in agent (worker) mode.
	Agents *int32 `json:"agents,omitempty"`
	// Token is the token used to join the worker nodes to the cluster.
	Token string `json:"token,omitempty"`
	// ClusterCIDR is the CIDR range for the pods of the cluster. Defaults to 10.42.0.0/16.
	ClusterCIDR string `json:"clusterCIDR,omitempty"`
	// ServiceCIDR is the CIDR range for the services in the cluster. Defaults to 10.43.0.0/16.
	ServiceCIDR string `json:"serviceCIDR,omitempty"`
	// ClusterDNS is the IP address for the coredns service. Needs to be in the range provided by ServiceCIDR or CoreDNS may not deploy.
	// Defaults to 10.43.0.10.
	ClusterDNS string `json:"clusterDNS,omitempty"`
	// ServerArgs are the ordered key value pairs (e.x. "testArg", "testValue") for the K3s pods running in server mode.
	ServerArgs []string `json:"serverArgs,omitempty"`
	// AgentArgs are the ordered key value pairs (e.x. "testArg", "testValue") for the K3s pods running in agent mode.
	AgentArgs []string `json:"agentArgs,omitempty"`
	// TLSSANs are the subjectAlternativeNames for the certificate the K3s server will use.
	TLSSANs []string `json:"tlsSANs,omitempty"`
	// Addons is a list of secrets containing raw YAML which will be deployed in the virtual K3k cluster on startup.
	Addons []upstream.Addon `json:"addons,omitempty"`
	// Persistence contains options controlling how the etcd data of the virtual cluster is persisted. By default, no data
	// persistence is guaranteed, so restart of a virtual cluster pod may result in data loss without this field.
	Persistence upstream.PersistenceConfig `json:"persistence,omitempty"`
	// Expose contains options for exposing the apiserver inside/outside of the cluster. By default, this is only exposed as a
	// clusterIP which is relatively secure, but difficult to access outside of the cluster.
	Expose *upstream.ExposeConfig `json:"expose,omitempty"`

	// Version is a string representing the Kubernetes version to be used by the virtual nodes.
	Version string `json:"version,omitempty"`
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
