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

package controller

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/hermes-operator/hermes-operator/api/v1"
)

const (
	defaultHermesImage     = "docker.io/nousresearch/hermes-agent:latest"
	defaultGatewayPort     = 8642
	defaultDashboardPort   = 9119
	hermesAgentFinalizer   = "hermesagent.finalizers.hermes.io"
	hermesConfigVolumeName = "hermes-config"
)

// HermesAgentReconciler reconciles a HermesAgent object
type HermesAgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

var log = logf.Log.WithName("controller_hermesagent")

// +kubebuilder:rbac:groups=core.hermes.io,resources=hermesagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.hermes.io,resources=hermesagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.hermes.io,resources=hermesagents/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles the main reconciliation logic for HermesAgent
func (r *HermesAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling HermesAgent")

	// Fetch the HermesAgent instance
	instance := &corev1alpha1.HermesAgent{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Handle deletion
	if !instance.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(ctx, instance)
	}

	// Add finalizer if not present
	if !containsFinalizer(instance, hermesAgentFinalizer) {
		return r.addFinalizer(ctx, instance)
	}

	// Reconcile the ConfigMap
	_, err = r.reconcileConfigMap(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the Pod
	_, err = r.reconcilePod(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the Service
	_, err = r.reconcileService(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Update status
	err = r.updateStatus(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}

// handleDeletion handles the deletion of HermesAgent
func (r *HermesAgentReconciler) handleDeletion(ctx context.Context, instance *corev1alpha1.HermesAgent) (ctrl.Result, error) {
	reqLogger := log.WithValues("HermesAgent.Name", instance.Name)
	reqLogger.Info("Handling deletion of HermesAgent")

	// Delete the associated ConfigMap
	configMapName := getConfigMapName(instance)
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: instance.Namespace}, configMap)
	if err == nil {
		reqLogger.Info("Deleting ConfigMap", "ConfigMap.Name", configMapName)
		if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Delete the associated Pod
	podName := getPodName(instance)
	pod := &corev1.Pod{}
	err = r.Get(ctx, client.ObjectKey{Name: podName, Namespace: instance.Namespace}, pod)
	if err == nil {
		reqLogger.Info("Deleting Pod", "Pod.Name", podName)
		if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Delete the associated Service
	svcName := getServiceName(instance)
	svc := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: svcName, Namespace: instance.Namespace}, svc)
	if err == nil {
		reqLogger.Info("Deleting Service", "Service.Name", svcName)
		if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Remove finalizer
	if containsFinalizer(instance, hermesAgentFinalizer) {
		instance.SetFinalizers(removeFinalizer(instance, hermesAgentFinalizer))
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// addFinalizer adds the finalizer to the HermesAgent
func (r *HermesAgentReconciler) addFinalizer(ctx context.Context, instance *corev1alpha1.HermesAgent) (ctrl.Result, error) {
	instance.SetFinalizers(append(instance.GetFinalizers(), hermesAgentFinalizer))
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// reconcileConfigMap creates or updates the ConfigMap for HermesAgent configuration files
func (r *HermesAgentReconciler) reconcileConfigMap(ctx context.Context, instance *corev1alpha1.HermesAgent) (ctrl.Result, error) {
	reqLogger := log.WithValues("HermesAgent.Name", instance.Name)
	configMapName := getConfigMapName(instance)

	// Build ConfigMap data
	configMapData := make(map[string]string)

	// Write .env file
	if len(instance.Spec.Env) > 0 {
		var envContent bytes.Buffer
		for key, value := range instance.Spec.Env {
			envContent.WriteString(fmt.Sprintf("%s=%s\n", key, value))
		}
		configMapData[".env"] = envContent.String()
	}

	// Write config.yaml
	if instance.Spec.ConfigYaml != "" {
		configMapData["config.yaml"] = instance.Spec.ConfigYaml
	}

	// Write SOUL.md
	if instance.Spec.SoulMd != "" {
		configMapData["SOUL.md"] = instance.Spec.SoulMd
	}

	// Get existing ConfigMap
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: instance.Namespace}, configMap)

	// Create ConfigMap if it doesn't exist
	if errors.IsNotFound(err) {
		configMap = r.createConfigMap(instance, configMapName, configMapData)
		if err := r.Create(ctx, configMap); err != nil {
			return ctrl.Result{}, err
		}
		reqLogger.Info("Created ConfigMap", "ConfigMap.Name", configMapName)
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	// Update ConfigMap if data changed
	if !reflect.DeepEqual(configMap.Data, configMapData) {
		configMap.Data = configMapData
		if err := r.Update(ctx, configMap); err != nil {
			return ctrl.Result{}, err
		}
		reqLogger.Info("Updated ConfigMap", "ConfigMap.Name", configMapName)
	}

	return ctrl.Result{}, nil
}

// createConfigMap creates a new ConfigMap for HermesAgent configuration
func (r *HermesAgentReconciler) createConfigMap(instance *corev1alpha1.HermesAgent, configMapName string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, corev1alpha1.GroupVersion.WithKind("HermesAgent")),
			},
		},
		Data: data,
	}
}

// reconcilePod creates or updates the Pod for HermesAgent
func (r *HermesAgentReconciler) reconcilePod(ctx context.Context, instance *corev1alpha1.HermesAgent) (ctrl.Result, error) {
	reqLogger := log.WithValues("HermesAgent.Name", instance.Name)
	podName := getPodName(instance)
	configMapName := getConfigMapName(instance)

	// Get existing Pod
	pod := &corev1.Pod{}
	err := r.Get(ctx, client.ObjectKey{Name: podName, Namespace: instance.Namespace}, pod)

	// Create Pod if it doesn't exist
	if errors.IsNotFound(err) {
		pod, err = r.createPod(ctx, instance, configMapName)
		if err != nil {
			return ctrl.Result{}, err
		}
		reqLogger.Info("Created Pod", "Pod.Name", pod.Name)
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	// Check if Pod needs to be updated
	if r.podNeedsUpdate(pod, instance, configMapName) {
		reqLogger.Info("Updating Pod", "Pod.Name", pod.Name)
		if err := r.Delete(ctx, pod); err != nil {
			return ctrl.Result{}, err
		}
		// Create new Pod with updated spec
		pod, err = r.createPod(ctx, instance, configMapName)
		if err != nil {
			return ctrl.Result{}, err
		}
		reqLogger.Info("Recreated Pod with new spec", "Pod.Name", pod.Name)
	}

	return ctrl.Result{}, nil
}

// createPod creates a new Pod for the HermesAgent
func (r *HermesAgentReconciler) createPod(ctx context.Context, instance *corev1alpha1.HermesAgent, configMapName string) (*corev1.Pod, error) {
	podName := getPodName(instance)

	// Get image
	image := instance.Spec.Image
	if image == "" {
		image = defaultHermesImage
	}

	// Get ports
	gatewayPort := instance.Spec.GatewayPort
	if gatewayPort == 0 {
		gatewayPort = defaultGatewayPort
	}
	dashboardPort := instance.Spec.DashboardPort
	if dashboardPort == 0 {
		dashboardPort = defaultDashboardPort
	}

	// Build environment variables for gateway
	gatewayEnvVars := r.buildGatewayEnvVars(instance, gatewayPort)

	// Build environment variables for dashboard
	dashboardEnvVars := r.buildDashboardEnvVars(instance, gatewayPort, dashboardPort)

	// Build gateway container
	gatewayContainer := corev1.Container{
		Name:            "gateway",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{"gateway", "run", "--verbose"},
		Ports: []corev1.ContainerPort{
			{
				Name:          "gateway",
				ContainerPort: int32(gatewayPort),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:          gatewayEnvVars,
		VolumeMounts: r.buildVolumeMounts(),
	}

	// Set gateway resource requirements
	if !reflect.DeepEqual(instance.Spec.GatewayResources, corev1.ResourceRequirements{}) {
		gatewayContainer.Resources = instance.Spec.GatewayResources
	} else {
		// Default resources for gateway
		gatewayContainer.Resources = corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("4Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1Gi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		}
	}

	// Build dashboard container
	dashboardContainer := corev1.Container{
		Name:            "dashboard",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{"dashboard", "--host", "0.0.0.0", "--insecure"},
		Ports: []corev1.ContainerPort{
			{
				Name:          "dashboard",
				ContainerPort: int32(dashboardPort),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:          dashboardEnvVars,
		VolumeMounts: r.buildVolumeMounts(),
	}

	// Set dashboard resource requirements
	if !reflect.DeepEqual(instance.Spec.DashboardResources, corev1.ResourceRequirements{}) {
		dashboardContainer.Resources = instance.Spec.DashboardResources
	} else {
		// Default resources for dashboard
		dashboardContainer.Resources = corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("512Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
		}
	}

	// Build Pod spec
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{gatewayContainer, dashboardContainer},
		Volumes:    r.buildVolumes(configMapName),
	}

	// Build Pod labels
	podLabels := map[string]string{
		"hermes.io/app":        instance.Name,
		"hermes.io/managed-by": "hermes-operator",
		"hermes.io/controller": "hermesagent",
	}

	// Build Pod annotations
	podAnnotations := map[string]string{
		"hermes.io/hermesagent": instance.Namespace + "/" + instance.Name,
	}

	// Create Pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   instance.Namespace,
			Labels:      podLabels,
			Annotations: podAnnotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, corev1alpha1.GroupVersion.WithKind("HermesAgent")),
			},
		},
		Spec: podSpec,
	}

	if err := ctrl.SetControllerReference(instance, pod, r.Scheme); err != nil {
		return nil, err
	}

	if err := r.Create(ctx, pod); err != nil {
		return nil, err
	}

	return pod, nil
}

// buildVolumeMounts builds the volume mounts for both containers
func (r *HermesAgentReconciler) buildVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      hermesConfigVolumeName,
			MountPath: "/opt/data",
			ReadOnly:  true,
		},
	}
}

// buildVolumes builds the volumes for the Pod
func (r *HermesAgentReconciler) buildVolumes(configMapName string) []corev1.Volume {
	return []corev1.Volume{
		{
			Name: hermesConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
				},
			},
		},
	}
}

// buildGatewayEnvVars builds environment variables for the gateway container
func (r *HermesAgentReconciler) buildGatewayEnvVars(instance *corev1alpha1.HermesAgent, port int) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "HERMES_SERVICE_PORT",
			Value: fmt.Sprintf("%d", port),
		},
		{
			Name: "HERMES_POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "HERMES_POD_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
		{
			Name: "HERMES_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
	}

	return envVars
}

// buildDashboardEnvVars builds environment variables for the dashboard container
func (r *HermesAgentReconciler) buildDashboardEnvVars(instance *corev1alpha1.HermesAgent, gatewayPort, dashboardPort int) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "GATEWAY_HEALTH_URL",
			Value: fmt.Sprintf("http://localhost:%d", gatewayPort),
		},
		{
			Name:  "DASHBOARD_PORT",
			Value: fmt.Sprintf("%d", dashboardPort),
		},
	}

	return envVars
}

// podNeedsUpdate checks if the Pod needs to be updated
func (r *HermesAgentReconciler) podNeedsUpdate(pod *corev1.Pod, instance *corev1alpha1.HermesAgent, configMapName string) bool {
	// Check image
	image := instance.Spec.Image
	if image == "" {
		image = defaultHermesImage
	}

	// Check if we have the expected containers
	if len(pod.Spec.Containers) != 2 {
		return true
	}

	// Check gateway container
	if pod.Spec.Containers[0].Name != "gateway" || pod.Spec.Containers[0].Image != image {
		return true
	}

	// Check dashboard container
	if pod.Spec.Containers[1].Name != "dashboard" || pod.Spec.Containers[1].Image != image {
		return true
	}

	// Check volumes reference ConfigMap
	if len(pod.Spec.Volumes) == 0 || pod.Spec.Volumes[0].ConfigMap == nil || pod.Spec.Volumes[0].ConfigMap.Name != configMapName {
		return true
	}

	// Check gateway port
	gatewayPort := instance.Spec.GatewayPort
	if gatewayPort == 0 {
		gatewayPort = defaultGatewayPort
	}
	if len(pod.Spec.Containers[0].Ports) == 0 || int(pod.Spec.Containers[0].Ports[0].ContainerPort) != gatewayPort {
		return true
	}

	// Check dashboard port
	dashboardPort := instance.Spec.DashboardPort
	if dashboardPort == 0 {
		dashboardPort = defaultDashboardPort
	}
	if len(pod.Spec.Containers[1].Ports) == 0 || int(pod.Spec.Containers[1].Ports[0].ContainerPort) != dashboardPort {
		return true
	}

	return false
}

// reconcileService creates or updates the Service for HermesAgent
func (r *HermesAgentReconciler) reconcileService(ctx context.Context, instance *corev1alpha1.HermesAgent) (ctrl.Result, error) {
	reqLogger := log.WithValues("HermesAgent.Name", instance.Name)
	svcName := getServiceName(instance)

	// Get ports
	gatewayPort := instance.Spec.GatewayPort
	if gatewayPort == 0 {
		gatewayPort = defaultGatewayPort
	}
	dashboardPort := instance.Spec.DashboardPort
	if dashboardPort == 0 {
		dashboardPort = defaultDashboardPort
	}

	// Get existing Service
	svc := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Name: svcName, Namespace: instance.Namespace}, svc)

	// Create Service if it doesn't exist
	if errors.IsNotFound(err) {
		svc = createService(instance, svcName, gatewayPort, dashboardPort)
		if err := r.Create(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
		reqLogger.Info("Created Service", "Service.Name", svcName)
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	// Update Service if needed
	svc = updateService(svc, instance, gatewayPort, dashboardPort)
	if err := r.Update(ctx, svc); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// createService creates a new Service for HermesAgent with both gateway and dashboard ports
func createService(instance *corev1alpha1.HermesAgent, svcName string, gatewayPort, dashboardPort int) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, corev1alpha1.GroupVersion.WithKind("HermesAgent")),
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:     "gateway",
					Port:     int32(gatewayPort),
					Protocol: corev1.ProtocolTCP,
					TargetPort: func() intstr.IntOrString {
						return intstr.FromInt(gatewayPort)
					}(),
				},
				{
					Name:     "dashboard",
					Port:     int32(dashboardPort),
					Protocol: corev1.ProtocolTCP,
					TargetPort: func() intstr.IntOrString {
						return intstr.FromInt(dashboardPort)
					}(),
				},
			},
			Selector: map[string]string{
				"hermes.io/app":        instance.Name,
				"hermes.io/managed-by": "hermes-operator",
			},
		},
	}
}

// updateService updates an existing Service
func updateService(svc *corev1.Service, instance *corev1alpha1.HermesAgent, gatewayPort, dashboardPort int) *corev1.Service {
	// Update selector to ensure it matches Pod
	svc.Spec.Selector = map[string]string{
		"hermes.io/app":        instance.Name,
		"hermes.io/managed-by": "hermes-operator",
	}

	// Ensure we have 2 ports
	if len(svc.Spec.Ports) != 2 {
		svc.Spec.Ports = make([]corev1.ServicePort, 2)
	}

	// Update gateway port
	svc.Spec.Ports[0].Name = "gateway"
	svc.Spec.Ports[0].Port = int32(gatewayPort)
	svc.Spec.Ports[0].Protocol = corev1.ProtocolTCP
	svc.Spec.Ports[0].TargetPort = intstr.FromInt(gatewayPort)

	// Update dashboard port
	svc.Spec.Ports[1].Name = "dashboard"
	svc.Spec.Ports[1].Port = int32(dashboardPort)
	svc.Spec.Ports[1].Protocol = corev1.ProtocolTCP
	svc.Spec.Ports[1].TargetPort = intstr.FromInt(dashboardPort)

	return svc
}

// updateStatus updates the status of HermesAgent
func (r *HermesAgentReconciler) updateStatus(ctx context.Context, instance *corev1alpha1.HermesAgent) error {
	podName := getPodName(instance)
	pod := &corev1.Pod{}
	err := r.Get(ctx, client.ObjectKey{Name: podName, Namespace: instance.Namespace}, pod)

	if err == nil {
		instance.Status.PodName = pod.Name
		instance.Status.PodIP = pod.Status.PodIP
		instance.Status.Phase = string(pod.Status.Phase)

		if instance.Status.StartedAt == nil && pod.Status.StartTime != nil {
			instance.Status.StartedAt = pod.Status.StartTime
		}
	}

	// Get ports
	gatewayPort := instance.Spec.GatewayPort
	if gatewayPort == 0 {
		gatewayPort = defaultGatewayPort
	}
	dashboardPort := instance.Spec.DashboardPort
	if dashboardPort == 0 {
		dashboardPort = defaultDashboardPort
	}
	instance.Status.GatewayPort = gatewayPort
	instance.Status.DashboardPort = dashboardPort

	// Update service info
	svcName := getServiceName(instance)
	svc := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: svcName, Namespace: instance.Namespace}, svc)
	if err == nil {
		instance.Status.ServiceName = svc.Name
		if len(svc.Spec.Ports) >= 2 {
			instance.Status.GatewayEndpoint = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
				svc.Name, svc.Namespace, svc.Spec.Ports[0].Port)
			instance.Status.DashboardEndpoint = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
				svc.Name, svc.Namespace, svc.Spec.Ports[1].Port)
		}
	}

	// Set ready condition
	readyCondition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             "PodNotReady",
		Message:            "Waiting for pod to be ready",
	}
	if err == nil && pod.Status.Phase == corev1.PodRunning {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "PodReady"
		readyCondition.Message = "Pod is running"
	}

	// Update or set condition
	found := false
	for i, c := range instance.Status.Conditions {
		if c.Type == "Ready" {
			instance.Status.Conditions[i] = readyCondition
			found = true
			break
		}
	}
	if !found {
		instance.Status.Conditions = append(instance.Status.Conditions, readyCondition)
	}

	return r.Status().Update(ctx, instance)
}

// getPodName returns the Pod name for a HermesAgent
func getPodName(instance *corev1alpha1.HermesAgent) string {
	return fmt.Sprintf("hermes-agent-%s", instance.Name)
}

// getServiceName returns the Service name for a HermesAgent
func getServiceName(instance *corev1alpha1.HermesAgent) string {
	return fmt.Sprintf("hermes-agent-%s", instance.Name)
}

// getConfigMapName returns the ConfigMap name for a HermesAgent
func getConfigMapName(instance *corev1alpha1.HermesAgent) string {
	return fmt.Sprintf("hermes-config-%s", instance.Name)
}

// containsFinalizer checks if the finalizer exists in the instance
func containsFinalizer(instance *corev1alpha1.HermesAgent, finalizer string) bool {
	for _, f := range instance.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

// removeFinalizer removes the finalizer from the instance
func removeFinalizer(instance *corev1alpha1.HermesAgent, finalizer string) []string {
	var finalizers []string
	for _, f := range instance.GetFinalizers() {
		if f != finalizer {
			finalizers = append(finalizers, f)
		}
	}
	return finalizers
}

// SetupWithManager sets up the controller with the Manager.
func (r *HermesAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.HermesAgent{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Named("hermesagent").
		Complete(r)
}

// formatEnvForDisplay formats env vars for display in comments (hiding sensitive values)
func formatEnvForDisplay(env map[string]string) string {
	var lines []string
	for key, value := range env {
		// Mask values that look like API keys
		if strings.Contains(strings.ToLower(key), "key") || strings.Contains(strings.ToLower(key), "secret") || strings.Contains(strings.ToLower(key), "token") {
			lines = append(lines, fmt.Sprintf("# %s=***", key))
		} else {
			lines = append(lines, fmt.Sprintf("# %s=%s", key, value))
		}
	}
	return strings.Join(lines, "\n")
}
