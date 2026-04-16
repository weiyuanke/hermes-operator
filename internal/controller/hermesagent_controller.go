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
	"context"
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	defaultHermesImage  = "ghcr.io/aisuko/hermes:latest"
	defaultServicePort  = 8000
	hermesAgentFinalizer = "hermesagent.finalizers.hermes.io"
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
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

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

	// Delete the associated Pod
	podName := getPodName(instance)
	pod := &corev1.Pod{}
	err := r.Get(ctx, client.ObjectKey{Name: podName, Namespace: instance.Namespace}, pod)
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

// reconcilePod creates or updates the Pod for HermesAgent
func (r *HermesAgentReconciler) reconcilePod(ctx context.Context, instance *corev1alpha1.HermesAgent) (ctrl.Result, error) {
	reqLogger := log.WithValues("HermesAgent.Name", instance.Name)
	podName := getPodName(instance)

	// Get existing Pod
	pod := &corev1.Pod{}
	err := r.Get(ctx, client.ObjectKey{Name: podName, Namespace: instance.Namespace}, pod)

	// Create Pod if it doesn't exist
	if errors.IsNotFound(err) {
		pod, err = r.createPod(ctx, instance)
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
	if r.podNeedsUpdate(pod, instance) {
		reqLogger.Info("Updating Pod", "Pod.Name", pod.Name)
		if err := r.Delete(ctx, pod); err != nil {
			return ctrl.Result{}, err
		}
		// Create new Pod with updated spec
		pod, err = r.createPod(ctx, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
		reqLogger.Info("Recreated Pod with new spec", "Pod.Name", pod.Name)
	}

	return ctrl.Result{}, nil
}

// createPod creates a new Pod for the HermesAgent
func (r *HermesAgentReconciler) createPod(ctx context.Context, instance *corev1alpha1.HermesAgent) (*corev1.Pod, error) {
	podName := getPodName(instance)

	// Get image
	image := instance.Spec.Image
	if image == "" {
		image = defaultHermesImage
	}

	// Get service port
	servicePort := instance.Spec.ServicePort
	if servicePort == 0 {
		servicePort = defaultServicePort
	}

	// Build environment variables
	envVars := r.buildEnvVars(instance, servicePort)

	// Build container
	container := corev1.Container{
		Name:            "hermes-agent",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: int32(servicePort),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: envVars,
	}

	// Set resource requirements
	if !reflect.DeepEqual(instance.Spec.Resources, corev1.ResourceRequirements{}) {
		container.Resources = instance.Spec.Resources
	}

	// Build Pod spec
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
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

// buildEnvVars builds environment variables for the container
func (r *HermesAgentReconciler) buildEnvVars(instance *corev1alpha1.HermesAgent, port int) []corev1.EnvVar {
	// Get secret key
	secretKey := instance.Spec.APISecretRef.Key
	if secretKey == "" {
		secretKey = "api-key"
	}

	// Get base URL
	baseURL := instance.Spec.BaseURL
	if baseURL == "" {
		baseURL = "https://api.moonshot.cn/v1"
	}

	// Get max turns
	maxTurns := instance.Spec.MaxTurns
	if maxTurns == 0 {
		maxTurns = 90
	}

	// Get personality
	personality := instance.Spec.Personality
	if personality == "" {
		personality = "kawaii"
	}

	envVars := []corev1.EnvVar{
		{
			Name:  "HERMES_MODEL",
			Value: instance.Spec.Model,
		},
		{
			Name:  "HERMES_PROVIDER",
			Value: instance.Spec.Provider,
		},
		{
			Name:  "HERMES_BASE_URL",
			Value: baseURL,
		},
		{
			Name:  "HERMES_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: instance.Spec.APISecretRef.Name,
					},
					Key:      secretKey,
					Optional: boolPtr(false),
				},
			},
		},
		{
			Name:  "HERMES_MAX_TURNS",
			Value: fmt.Sprintf("%d", maxTurns),
		},
		{
			Name:  "HERMES_PERSONALITY",
			Value: personality,
		},
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

// podNeedsUpdate checks if the Pod needs to be updated
func (r *HermesAgentReconciler) podNeedsUpdate(pod *corev1.Pod, instance *corev1alpha1.HermesAgent) bool {
	// Check image
	image := instance.Spec.Image
	if image == "" {
		image = defaultHermesImage
	}
	if len(pod.Spec.Containers) > 0 && pod.Spec.Containers[0].Image != image {
		return true
	}

	// Check resource requirements
	if len(pod.Spec.Containers) > 0 && !reflect.DeepEqual(pod.Spec.Containers[0].Resources, instance.Spec.Resources) {
		return true
	}

	return false
}

// reconcileService creates or updates the Service for HermesAgent
func (r *HermesAgentReconciler) reconcileService(ctx context.Context, instance *corev1alpha1.HermesAgent) (ctrl.Result, error) {
	reqLogger := log.WithValues("HermesAgent.Name", instance.Name)
	svcName := getServiceName(instance)

	// Get service port
	servicePort := instance.Spec.ServicePort
	if servicePort == 0 {
		servicePort = defaultServicePort
	}

	// Get existing Service
	svc := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Name: svcName, Namespace: instance.Namespace}, svc)

	// Create Service if it doesn't exist
	if errors.IsNotFound(err) {
		svc = createService(instance, svcName, servicePort)
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
	svc = updateService(svc, instance, servicePort)
	if err := r.Update(ctx, svc); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// createService creates a new Service for HermesAgent
func createService(instance *corev1alpha1.HermesAgent, svcName string, port int) *corev1.Service {
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
					Name:     "http",
					Port:     int32(port),
					Protocol: corev1.ProtocolTCP,
					TargetPort: func() intstr.IntOrString {
						return intstr.FromInt(port)
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
func updateService(svc *corev1.Service, instance *corev1alpha1.HermesAgent, port int) *corev1.Service {
	// Update selector to ensure it matches Pod
	svc.Spec.Selector = map[string]string{
		"hermes.io/app":        instance.Name,
		"hermes.io/managed-by": "hermes-operator",
	}

	// Update port if needed
	if len(svc.Spec.Ports) > 0 {
		svc.Spec.Ports[0].Port = int32(port)
	}

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

	// Update service info
	svcName := getServiceName(instance)
	svc := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: svcName, Namespace: instance.Namespace}, svc)
	if err == nil {
		instance.Status.ServiceName = svc.Name
		if len(svc.Spec.Ports) > 0 {
			instance.Status.ServicePort = int(svc.Spec.Ports[0].Port)
			instance.Status.Endpoint = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
				svc.Name, svc.Namespace, svc.Spec.Ports[0].Port)
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

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}

// SetupWithManager sets up the controller with the Manager.
func (r *HermesAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.HermesAgent{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Named("hermesagent").
		Complete(r)
}
