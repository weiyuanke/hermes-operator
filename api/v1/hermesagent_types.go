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
	// +kubebuilder:default="docker.io/nousresearch/hermes-agent:v2026.4.16"
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

	// Env defines the raw content of the .env file.
	// This will be written to /opt/data/.env in the container.
	// +optional
	Env string `json:"env,omitempty"`

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

	// StorageClassName is the storage class for the PVC that backs /opt/data.
	// If empty, the cluster default storage class is used.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// StorageSize is the size of the PVC that backs /opt/data.
	// +optional
	// +kubebuilder:default="1Gi"
	StorageSize string `json:"storageSize,omitempty"`

	// WebUIPort is the port where the Open WebUI container will listen.
	// When set to 0 (default), the Open WebUI container is not deployed.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	WebUIPort int `json:"webUIPort,omitempty"`

	// WebUIApiKey is the API key used by Open WebUI to authenticate against the gateway.
	// Corresponds to OPENAI_API_KEY in the Open WebUI container.
	// +optional
	WebUIApiKey string `json:"webUIApiKey,omitempty"`

	// WebUIResources defines the compute resources for the Open WebUI container.
	// +optional
	WebUIResources corev1.ResourceRequirements `json:"webUIResources,omitempty"`
}

// HermesAgentStatus defines the observed state of HermesAgent
type HermesAgentStatus struct {
	// Phase is the current phase of the Hermes agent.
	// +optional
	Phase string `json:"phase,omitempty"`

	// ServiceName is the name of the ClusterIP Service exposing the Hermes agent internally.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// LoadBalancerServiceName is the name of the LoadBalancer Service exposing the Hermes agent externally.
	// +optional
	LoadBalancerServiceName string `json:"loadBalancerServiceName,omitempty"`

	// LoadBalancerIngress is the external IP or hostname assigned to the LoadBalancer Service.
	// +optional
	LoadBalancerIngress string `json:"loadBalancerIngress,omitempty"`

	// GatewayPort is the port where the gateway service is exposed.
	// +optional
	GatewayPort int `json:"gatewayPort,omitempty"`

	// DashboardPort is the port where the dashboard service is exposed.
	// +optional
	DashboardPort int `json:"dashboardPort,omitempty"`

	// DeploymentName is the name of the Deployment running the Hermes agent.
	// +optional
	DeploymentName string `json:"deploymentName,omitempty"`

	// ReadyReplicas is the number of ready replicas in the Deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// GatewayEndpoint is the HTTP endpoint where the Hermes gateway can be accessed.
	// +optional
	GatewayEndpoint string `json:"gatewayEndpoint,omitempty"`

	// DashboardEndpoint is the HTTP endpoint where the Hermes dashboard can be accessed.
	// +optional
	DashboardEndpoint string `json:"dashboardEndpoint,omitempty"`

	// WebUIEndpoint is the HTTP endpoint where the Open WebUI can be accessed.
	// Empty when WebUIPort is not configured.
	// +optional
	WebUIEndpoint string `json:"webUIEndpoint,omitempty"`

	// StartedAt is the time when the Hermes agent Deployment was created.
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
