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
	upstream "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// K3kControlPlaneSpec defines the desired state of K3kControlPlane
type K3kControlPlaneSpec struct {
	// HostKubeconfig is the location of the kubeconfig to the host cluster that the k3k cluster should install in.
	// Optional, if not supplied the k3k cluster will be made in the current cluster.
	HostKubeconfig *HostKubeconfigLocation `json:"hostKubeconfig,omitempty"`

	// The following fields are copied from the github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1.ClusterSpec

	Servers     *int32                      `json:"servers"`
	Agents      *int32                      `json:"agents"`
	Token       string                      `json:"token"`
	ClusterCIDR string                      `json:"clusterCIDR,omitempty"`
	ServiceCIDR string                      `json:"serviceCIDR,omitempty"`
	ClusterDNS  string                      `json:"clusterDNS,omitempty"`
	ServerArgs  []string                    `json:"serverArgs,omitempty"`
	AgentArgs   []string                    `json:"agentArgs,omitempty"`
	TLSSANs     []string                    `json:"tlsSANs,omitempty"`
	Addons      []upstream.Addon            `json:"addons,omitempty"`
	Persistence *upstream.PersistenceConfig `json:"persistence,omitempty"`
	Expose      *upstream.ExposeConfig      `json:"expose,omitempty"`

	// Version is a string representing the Kubernetes version to be used by the virtual nodes.
	Version string `json:"version"`
}

type HostKubeconfigLocation struct {
	SecretName      string `json:"secretName"`
	SecretNamespace string `json:"secretNamespace"`
}

// K3kControlPlaneStatus defines the observed state of K3kControlPlane
type K3kControlPlaneStatus struct {
	Initialized bool `json:"initialized"`
	Ready       bool `json:"ready"`
	// ExternalManagedControlPlane will always be true in the current implementation
	ExternalManagedControlPlane bool                   `json:"externalManagedControlPlane"`
	ClusterStatus               upstream.ClusterStatus `json:"clusterStatus"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// K3kControlPlane is the Schema for the k3kcontrolplanes API
type K3kControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   K3kControlPlaneSpec   `json:"spec,omitempty"`
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
