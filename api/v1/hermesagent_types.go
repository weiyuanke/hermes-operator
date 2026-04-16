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
	// +optional
	// +kubebuilder:default="docker.io/nousresearch/hermes-agent:latest"
	Image string `json:"image,omitempty"`

	// GatewayPort is the port where the Hermes gateway service will listen.
	// +optional
	// +kubebuilder:default=8642
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	GatewayPort int `json:"gatewayPort,omitempty"`

	// DashboardPort is the port where the Hermes dashboard service will listen.
	// +optional
	// +kubebuilder:default=9119
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	DashboardPort int `json:"dashboardPort,omitempty"`

	// Env defines environment variables to be written to .env file.
	// These will be written to /opt/data/.env in the container.
	// +optional
	Env map[string]string `json:"env,omitempty"`

	// ConfigYaml defines the content of config.yaml file.
	// This will be written to /opt/data/config.yaml in the container.
	// +optional
	ConfigYaml string `json:"configYaml,omitempty"`

	// SoulMd defines the content of SOUL.md file.
	// This will be written to /opt/data/SOUL.md in the container.
	// +optional
	SoulMd string `json:"soulMd,omitempty"`

	// GatewayResources defines the compute resources for the gateway container.
	// +optional
	GatewayResources corev1.ResourceRequirements `json:"gatewayResources,omitempty"`

	// DashboardResources defines the compute resources for the dashboard container.
	// +optional
	DashboardResources corev1.ResourceRequirements `json:"dashboardResources,omitempty"`
}

// HermesAgentStatus defines the observed state of HermesAgent
type HermesAgentStatus struct {
	// Phase is the current phase of the Hermes agent.
	// +optional
	Phase string `json:"phase,omitempty"`

	// ServiceName is the name of the Service exposing the Hermes agent.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// GatewayPort is the port where the gateway service is exposed.
	// +optional
	GatewayPort int `json:"gatewayPort,omitempty"`

	// DashboardPort is the port where the dashboard service is exposed.
	// +optional
	DashboardPort int `json:"dashboardPort,omitempty"`

	// PodName is the name of the Hermes agent Pod.
	// +optional
	PodName string `json:"podName,omitempty"`

	// PodIP is the IP address of the Hermes agent Pod.
	// +optional
	PodIP string `json:"podIP,omitempty"`

	// GatewayEndpoint is the HTTP endpoint where the Hermes gateway can be accessed.
	// +optional
	GatewayEndpoint string `json:"gatewayEndpoint,omitempty"`

	// DashboardEndpoint is the HTTP endpoint where the Hermes dashboard can be accessed.
	// +optional
	DashboardEndpoint string `json:"dashboardEndpoint,omitempty"`

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
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="Current phase"
// +kubebuilder:printcolumn:name="Gateway",type="string",JSONPath=".status.gatewayEndpoint",description="Gateway endpoint"
// +kubebuilder:printcolumn:name="Dashboard",type="string",JSONPath=".status.dashboardEndpoint",description="Dashboard endpoint"
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
