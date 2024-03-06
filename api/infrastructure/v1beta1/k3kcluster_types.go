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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// K3kClusterSpec defines the desired state of K3kCluster
type K3kClusterSpec struct {
	// ControlPlaneEndpoint is the endpoint that the server can be reached at. Should not be supplied at create
	// time by the end user, the controller will fill this in when provisioning is complete.
	ControlPlaneEndpoint ApiEndpoint `json:"controlPlaneEndpoint"`
}

type ApiEndpoint struct {
	Host string `json:"host"`
	Port int32  `json:"port"`
}

// K3kClusterStatus defines the observed state of K3kCluster
type K3kClusterStatus struct {
	// Ready is true when the cluster is ready to be used
	Ready bool `json:"ready"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// K3kCluster is the Schema for the k3kclusters API
type K3kCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   K3kClusterSpec   `json:"spec,omitempty"`
	Status K3kClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// K3kClusterList contains a list of K3kCluster
type K3kClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K3kCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&K3kCluster{}, &K3kClusterList{})
}
