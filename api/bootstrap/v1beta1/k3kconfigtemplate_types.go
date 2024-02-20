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

// K3kConfigTemplateSpec defines the desired state of K3kConfigTemplate
type K3kConfigTemplateSpec struct {
	// ServerConfig is the config/args for k3s running in a server configuration.
	// One and only one of ServerConfig and AgentConfig must be provided. Will deploy
	// in server mode if provided.
	ServerConfig *ServerConfig `json:"serverConfig,omitempty"`

	// AgentConfig is the config/args for k3s running in an agent configuration.
	// One and only one of ServerConfig and AgentConfig must be provided. Will deploy
	// in agent mode if provided.
	AgentConfig *AgentConfig `json:"agentConfig,omitempty"`
}

type ServerConfig struct {
	ServerArgs []string `json:"serverArgs,omitempty"`
}

type AgentConfig struct {
	AgentArgs []string `json:"serverArgs,omitempty"`
}

// K3kConfigTemplateStatus defines the observed state of K3kConfigTemplate
type K3kConfigTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// K3kConfigTemplate is the Schema for the k3kconfigtemplates API
type K3kConfigTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   K3kConfigTemplateSpec   `json:"spec,omitempty"`
	Status K3kConfigTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// K3kConfigTemplateList contains a list of K3kConfigTemplate
type K3kConfigTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K3kConfigTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&K3kConfigTemplate{}, &K3kConfigTemplateList{})
}
