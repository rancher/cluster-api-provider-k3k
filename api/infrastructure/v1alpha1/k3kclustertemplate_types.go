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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// K3kClusterTemplateSpec defines the desired state of K3kClusterTemplate
type K3kClusterTemplateSpec struct {
	Template K3kClusterTemplateConfig `json:"template"`
}

type K3kClusterTemplateConfig struct {
	ObjectMeta clusterv1beta1.ObjectMeta `json:"metadata,omitempty"`
	Spec       K3kClusterSpec            `json:"spec"`
}

// K3kClusterTemplateStatus defines the observed state of K3kClusterTemplate
type K3kClusterTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// K3kClusterTemplate is the Schema for the k3kclustertemplates API
type K3kClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   K3kClusterTemplateSpec   `json:"spec,omitempty"`
	Status K3kClusterTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// K3kClusterTemplateList contains a list of K3kClusterTemplate
type K3kClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K3kClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&K3kClusterTemplate{}, &K3kClusterTemplateList{})
}
