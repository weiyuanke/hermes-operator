/*
Copyright 2026.

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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HermesAgentSpec defines the desired state of HermesAgent
type HermesAgentSpec struct {
	// Model is the AI model name to use.
	// Examples: "kimi-k2.5", "gpt-4", "claude-3-sonnet"
	// +required
	Model string `json:"model"`

	// Provider is the model provider name.
	// Examples: "kimi-coding-cn", "openai", "openrouter"
	// +required
	Provider string `json:"provider"`

	// BaseURL is the API endpoint for the model provider.
	// Examples: "https://api.moonshot.cn/v1", "https://api.openai.com/v1"
	// +optional
	// +kubebuilder:default="https://api.moonshot.cn/v1"
	BaseURL string `json:"baseURL,omitempty"`

	// APISecretRef is the reference to a Kubernetes Secret containing the API key.
	// The secret should have a key named "api-key".
	// +required
	APISecretRef SecretRef `json:"apiSecretRef"`

	// MaxTurns is the maximum number of conversation turns.
	// +optional
	// +kubebuilder:default=90
	MaxTurns int `json:"maxTurns,omitempty"`

	// Personality is the agent's personality preset.
	// +optional
	// +kubebuilder:default="kawaii"
	Personality string `json:"personality,omitempty"`

	// Image is the container image for Hermes agent.
	// +optional
	// +kubebuilder:default="docker.io/nousresearch/hermes-agent:latest"
	Image string `json:"image,omitempty"`

	// ServicePort is the port where the Hermes agent service will listen.
	// +optional
	// +kubebuilder:default=8000
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ServicePort int `json:"servicePort,omitempty"`

	// Resources defines the compute resources for the Hermes agent pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// SecretRef is a reference to a Kubernetes Secret
type SecretRef struct {
	// Name is the name of the Secret.
	// +required
	Name string `json:"name"`

	// Namespace is the namespace of the Secret.
	// If not specified, the same namespace as the HermesAgent is used.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Key is the key in the Secret data. Defaults to "api-key".
	// +optional
	// +kubebuilder:default="api-key"
	Key string `json:"key,omitempty"`
}

// HermesAgentStatus defines the observed state of HermesAgent
type HermesAgentStatus struct {
	// Phase is the current phase of the Hermes agent.
	// +optional
	Phase string `json:"phase,omitempty"`

	// ServiceName is the name of the Service exposing the Hermes agent.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// ServicePort is the port where the Hermes agent service is exposed.
	// +optional
	ServicePort int `json:"servicePort,omitempty"`

	// PodName is the name of the Hermes agent Pod.
	// +optional
	PodName string `json:"podName,omitempty"`

	// PodIP is the IP address of the Hermes agent Pod.
	// +optional
	PodIP string `json:"podIP,omitempty"`

	// Endpoint is the HTTP endpoint where the Hermes agent can be accessed.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// StartedAt is the time when the Hermes agent was started.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// conditions represent the current state of the HermesAgent resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Model",type="string",JSONPath=".spec.model",description="AI model"
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=".spec.provider",description="Model provider"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="Current phase"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".status.endpoint",description="Service endpoint"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// HermesAgent is the Schema for the hermesagents API
type HermesAgent struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of HermesAgent
	// +required
	Spec HermesAgentSpec `json:"spec"`

	// status defines the observed state of HermesAgent
	// +optional
	Status HermesAgentStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// HermesAgentList contains a list of HermesAgent
type HermesAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []HermesAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HermesAgent{}, &HermesAgentList{})
}
