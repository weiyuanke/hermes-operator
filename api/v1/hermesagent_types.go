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
	// Image is the container image for Hermes agent.
	// If not specified, a default image will be used.
	// +optional
	Image string `json:"image,omitempty"`

	// ImagePullPolicy is the image pull policy for the container.
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Replicas is the number of Hermes agent pods to run.
	// Currently only supports 1 (one Hermes agent per CR).
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1
	Replicas int32 `json:"replicas,omitempty"`

	// Resources defines the compute resources for the Hermes agent pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// ServicePort is the port where the Hermes agent service will listen.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ServicePort int32 `json:"servicePort,omitempty"`

	// Config contains custom configuration for the Hermes agent.
	// +optional
	Config map[string]string `json:"config,omitempty"`

	// ServiceAccountName is the name of the ServiceAccount to use for the Pod.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// NodeSelector is a map of key-value pairs to select the node for the Pod.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations allows the Pod to be scheduled on nodes with taints.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity defines the scheduling constraints for the Pod.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Volumes defines additional volumes to mount into the container.
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// VolumeMounts defines the volume mounts for the container.
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// Labels are additional labels to add to the Pod.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are additional annotations to add to the Pod.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// HermesConfig is the configuration for the Hermes agent itself.
	// +optional
	HermesConfig *HermesConfigSpec `json:"hermesConfig,omitempty"`
}

// HermesConfigSpec defines the Hermes agent configuration
type HermesConfigSpec struct {
	// Model is the AI model to use (e.g., "gpt-4", "claude-3", etc.).
	// +optional
	Model string `json:"model,omitempty"`

	// APIKeySecretRef is the reference to a Kubernetes Secret containing the API key.
	// The secret should have a key named "api-key".
	// +optional
	APIKeySecretRef *SecretRef `json:"apiKeySecretRef,omitempty"`

	// Tools is the list of tools enabled for the Hermes agent.
	// +optional
	Tools []string `json:"tools,omitempty"`

	// MaxIterations is the maximum number of iterations for the agent.
	// +optional
	MaxIterations int32 `json:"maxIterations,omitempty"`

	// SystemPrompt is the system prompt for the Hermes agent.
	// +optional
	SystemPrompt string `json:"systemPrompt,omitempty"`
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
}

// HermesAgentStatus defines the observed state of HermesAgent.
type HermesAgentStatus struct {
	// PodName is the name of the Hermes agent Pod.
	// +optional
	PodName string `json:"podName,omitempty"`

	// Phase is the current phase of the Hermes agent.
	// +optional
	Phase string `json:"phase,omitempty"`

	// ServiceName is the name of the Service exposing the Hermes agent.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// ServicePort is the port where the Hermes agent service is exposed.
	// +optional
	ServicePort int32 `json:"servicePort,omitempty"`

	// Endpoint is the HTTP endpoint where the Hermes agent can be accessed.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// PodIP is the IP address of the Hermes agent Pod.
	// +optional
	PodIP string `json:"podIP,omitempty"`

	// StartedAt is the time when the Hermes agent was started.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// ReadyReplicas is the number of ready Hermes agent replicas.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// conditions represent the current state of the HermesAgent resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="The current phase of the Hermes agent"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas",description="Number of ready replicas"
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
