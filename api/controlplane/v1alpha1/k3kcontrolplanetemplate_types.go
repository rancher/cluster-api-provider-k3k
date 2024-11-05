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
// Important: Run "make" to regenerate code after modifying this file

// K3kControlPlaneTemplateSpec defines the desired state of K3kControlPlaneTemplate
type K3kControlPlaneTemplateSpec struct {
	Template K3kControlPlaneTemplateConfig `json:"template"`
}

type K3kControlPlaneTemplateConfig struct {
	ObjectMeta clusterv1beta1.ObjectMeta `json:"metadata,omitempty"`
	Spec       K3kControlPlaneSpec       `json:"spec"`
}

// K3kControlPlaneTemplateStatus defines the observed state of K3kControlPlaneTemplate
type K3kControlPlaneTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// K3kControlPlaneTemplate is the Schema for the k3kcontrolplanetemplates API
type K3kControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   K3kControlPlaneTemplateSpec   `json:"spec,omitempty"`
	Status K3kControlPlaneTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// K3kControlPlaneTemplateList contains a list of K3kControlPlaneTemplate
type K3kControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K3kControlPlaneTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&K3kControlPlaneTemplate{}, &K3kControlPlaneTemplateList{})
}
