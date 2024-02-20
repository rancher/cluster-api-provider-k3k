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
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Version is the k8s version of the k3k cluster.
	Version string `json:"version"`
	// ClusterCIDR is the CIDR for the cluster.
	ClusterCIDR string `json:"clusterCIDR,omitempty"`
	// ServiceCIDR is the CIDR for the services.
	ServiceCIDR string `json:"serviceCIDR,omitempty"`
	// ClusterDNS is the cluster DNS.
	ClusterDNS string `json:"clusterDNS,omitempty"`
	// TLSSANs is the SANs for the TLS cert the cluster will use.
	TLSSANs []string `json:"tlsSANs,omitempty"`
	// Addons are the addons which will also be deployed on the cluster.
	Addons []upstream.Addon `json:"addons,omitempty"`
}

// K3kControlPlaneStatus defines the observed state of K3kControlPlane
type K3kControlPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
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
