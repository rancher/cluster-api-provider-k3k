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

// K3kClusterSpec defines the desired state of K3kCluster
type K3kClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// HostKubeconfig is the location of the kubeconfig to the host cluster that the k3k
	// cluster should install in.
	HostKubeconfig HostKubeconfigLocation `json:"hostKubeconfig"`
	// Persistence is the configuration used to persist k3k data accross potential reboots.
	Persistence *upstream.PersistenceConfig `json:"persistence,omitempty"`
	// Expose is the config used to expose a k3k cluster outside of the host cluster.
	Expose *upstream.ExposeConfig `json:"expose,omitempty"`
}

type HostKubeconfigLocation struct {
	SecretName      string `json:"secretName"`
	SecretNamespace string `json:"secretNamespace"`
}

// K3kClusterStatus defines the observed state of K3kCluster
type K3kClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
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
