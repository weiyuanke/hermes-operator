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

	appsv1 "k8s.io/api/apps/v1"
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
	defaultHermesImage     = "docker.io/nousresearch/hermes-agent:v2026.4.16"
	defaultGatewayPort     = 8642
	defaultDashboardPort   = 9119
	defaultStorageSize     = "1Gi"
	hermesAgentFinalizer   = "hermesagent.finalizers.hermes.io"
	hermesDataVolumeName   = "hermes-data"
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
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

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

	// Reconcile the PVC (create-only; never updated after creation)
	if err := r.reconcilePVC(ctx, instance); err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the ConfigMap (create-only; never updated after creation)
	if err := r.reconcileConfigMap(ctx, instance); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.reconcileDeployment(ctx, instance); err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the Service
	if _, err := r.reconcileService(ctx, instance); err != nil {
		return reconcile.Result{}, err
	}

	// Update status
	if err := r.updateStatus(ctx, instance); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}

// handleDeletion handles the deletion of HermesAgent
func (r *HermesAgentReconciler) handleDeletion(ctx context.Context, instance *corev1alpha1.HermesAgent) (ctrl.Result, error) {
	reqLogger := log.WithValues("HermesAgent.Name", instance.Name)
	reqLogger.Info("Handling deletion of HermesAgent")

	// Delete the associated Deployment
	deployName := getDeploymentName(instance)
	deploy := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: deployName, Namespace: instance.Namespace}, deploy)
	if err == nil {
		reqLogger.Info("Deleting Deployment", "Deployment.Name", deployName)
		if err := r.Delete(ctx, deploy); err != nil && !errors.IsNotFound(err) {
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

	// Delete the associated ConfigMap
	configMapName := getConfigMapName(instance)
	configMap := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: instance.Namespace}, configMap)
	if err == nil {
		reqLogger.Info("Deleting ConfigMap", "ConfigMap.Name", configMapName)
		if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Delete the associated PVC
	pvcName := getPVCName(instance)
	pvc := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: instance.Namespace}, pvc)
	if err == nil {
		reqLogger.Info("Deleting PVC", "PVC.Name", pvcName)
		if err := r.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
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

// reconcileConfigMap creates the ConfigMap holding Env/ConfigYaml/SoulMd if it
// does not exist. The ConfigMap is never updated after creation — it is only
// used by the init container to seed the PVC on first boot.
func (r *HermesAgentReconciler) reconcileConfigMap(ctx context.Context, instance *corev1alpha1.HermesAgent) error {
	reqLogger := log.WithValues("HermesAgent.Name", instance.Name)
	configMapName := getConfigMapName(instance)

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: instance.Namespace}, cm)
	if err == nil {
		// ConfigMap already exists; do not update
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	data := make(map[string]string)
	if instance.Spec.Env != "" {
		data[".env"] = instance.Spec.Env
	}
	if instance.Spec.ConfigYaml != "" {
		data["config.yaml"] = instance.Spec.ConfigYaml
	}
	if instance.Spec.SoulMd != "" {
		data["SOUL.md"] = instance.Spec.SoulMd
	}

	newCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, corev1alpha1.GroupVersion.WithKind("HermesAgent")),
			},
		},
		Data: data,
	}
	if err := r.Create(ctx, newCM); err != nil {
		return err
	}
	reqLogger.Info("Created ConfigMap", "ConfigMap.Name", configMapName)
	return nil
}

// reconcilePVC creates the PVC for /opt/data if it does not exist.
// The PVC is never updated after creation.
func (r *HermesAgentReconciler) reconcilePVC(ctx context.Context, instance *corev1alpha1.HermesAgent) error {
	reqLogger := log.WithValues("HermesAgent.Name", instance.Name)
	pvcName := getPVCName(instance)

	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: instance.Namespace}, pvc)
	if err == nil {
		// PVC already exists; do not update
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	// Determine storage size
	storageSize := instance.Spec.StorageSize
	if storageSize == "" {
		storageSize = defaultStorageSize
	}
	storageQty, err := resource.ParseQuantity(storageSize)
	if err != nil {
		return fmt.Errorf("invalid storageSize %q: %w", storageSize, err)
	}

	newPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, corev1alpha1.GroupVersion.WithKind("HermesAgent")),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageQty,
				},
			},
		},
	}

	// Set storage class if specified
	if instance.Spec.StorageClassName != "" {
		newPVC.Spec.StorageClassName = &instance.Spec.StorageClassName
	}

	if err := r.Create(ctx, newPVC); err != nil {
		return err
	}
	reqLogger.Info("Created PVC", "PVC.Name", pvcName)
	return nil
}

// reconcileDeployment creates the Deployment if it does not exist.
// The Deployment is never updated after creation — changes to the CR spec have
// no effect until the CR is deleted and recreated.
func (r *HermesAgentReconciler) reconcileDeployment(ctx context.Context, instance *corev1alpha1.HermesAgent) error {
	reqLogger := log.WithValues("HermesAgent.Name", instance.Name)
	deployName := getDeploymentName(instance)

	deploy := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: deployName, Namespace: instance.Namespace}, deploy)
	if err == nil {
		// Deployment already exists; do not update
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	deploy, err = r.buildDeployment(instance)
	if err != nil {
		return err
	}
	if err := r.Create(ctx, deploy); err != nil {
		return err
	}
	reqLogger.Info("Created Deployment", "Deployment.Name", deploy.Name)
	return nil
}

// buildDeployment constructs the Deployment spec. The init container copies
// config files from the ConfigMap into the PVC on first boot (skipping files
// that already exist so restarts are idempotent).
func (r *HermesAgentReconciler) buildDeployment(instance *corev1alpha1.HermesAgent) (*appsv1.Deployment, error) {
	deployName := getDeploymentName(instance)
	pvcName := getPVCName(instance)

	image := instance.Spec.Image
	if image == "" {
		image = defaultHermesImage
	}

	gatewayPort := instance.Spec.GatewayPort
	if gatewayPort == 0 {
		gatewayPort = defaultGatewayPort
	}
	dashboardPort := instance.Spec.DashboardPort
	if dashboardPort == 0 {
		dashboardPort = defaultDashboardPort
	}

	podLabels := map[string]string{
		"hermes.io/app":        instance.Name,
		"hermes.io/managed-by": "hermes-operator",
		"hermes.io/controller": "hermesagent",
	}

	// init container: copy each config file into the PVC only if it does not
	// already exist there, then set 644 permissions.
	initContainer := corev1.Container{
		Name:            "init-config",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{
			"sh", "-c",
			"for f in /config/*; do dst=/opt/data/$(basename $f); [ -e $dst ] || cp $f $dst; chmod 644 $dst; done",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      hermesConfigVolumeName,
				MountPath: "/config",
				ReadOnly:  true,
			},
			{
				Name:      hermesDataVolumeName,
				MountPath: "/opt/data",
			},
		},
	}

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
		Env:          buildGatewayEnvVars(gatewayPort),
		VolumeMounts: buildDataVolumeMounts(),
	}

	if !reflect.DeepEqual(instance.Spec.GatewayResources, corev1.ResourceRequirements{}) {
		gatewayContainer.Resources = instance.Spec.GatewayResources
	} else {
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
		Env:          buildDashboardEnvVars(gatewayPort, dashboardPort),
		VolumeMounts: buildDataVolumeMounts(),
	}

	if !reflect.DeepEqual(instance.Spec.DashboardResources, corev1.ResourceRequirements{}) {
		dashboardContainer.Resources = instance.Spec.DashboardResources
	} else {
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

	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: instance.Namespace,
			Labels:    podLabels,
			Annotations: map[string]string{
				"hermes.io/hermesagent": instance.Namespace + "/" + instance.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, corev1alpha1.GroupVersion.WithKind("HermesAgent")),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: podLabels,
			},
			// Recreate strategy: PVC is RWO, so only one Pod can mount it at a time.
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
					Annotations: map[string]string{
						"hermes.io/hermesagent": instance.Namespace + "/" + instance.Name,
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{initContainer},
					Containers:     []corev1.Container{gatewayContainer, dashboardContainer},
					Volumes: []corev1.Volume{
						{
							Name: hermesDataVolumeName,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
						{
							Name: hermesConfigVolumeName,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: getConfigMapName(instance),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(instance, deploy, r.Scheme); err != nil {
		return nil, err
	}
	return deploy, nil
}

// buildDataVolumeMounts returns the volume mount for /opt/data used by main containers.
func buildDataVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      hermesDataVolumeName,
			MountPath: "/opt/data",
		},
	}
}

// buildGatewayEnvVars builds environment variables for the gateway container.
func buildGatewayEnvVars(port int) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "HERMES_SERVICE_PORT",
			Value: fmt.Sprintf("%d", port),
		},
		{
			Name: "HERMES_POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
			},
		},
		{
			Name: "HERMES_POD_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"},
			},
		},
		{
			Name: "HERMES_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
			},
		},
	}
}

// buildDashboardEnvVars builds environment variables for the dashboard container.
func buildDashboardEnvVars(gatewayPort, dashboardPort int) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "GATEWAY_HEALTH_URL",
			Value: fmt.Sprintf("http://localhost:%d", gatewayPort),
		},
		{
			Name:  "DASHBOARD_PORT",
			Value: fmt.Sprintf("%d", dashboardPort),
		},
	}
}

// reconcileService creates or updates the Service for HermesAgent.
func (r *HermesAgentReconciler) reconcileService(ctx context.Context, instance *corev1alpha1.HermesAgent) (ctrl.Result, error) {
	reqLogger := log.WithValues("HermesAgent.Name", instance.Name)
	svcName := getServiceName(instance)

	gatewayPort := instance.Spec.GatewayPort
	if gatewayPort == 0 {
		gatewayPort = defaultGatewayPort
	}
	dashboardPort := instance.Spec.DashboardPort
	if dashboardPort == 0 {
		dashboardPort = defaultDashboardPort
	}

	svc := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Name: svcName, Namespace: instance.Namespace}, svc)

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

	svc = updateService(svc, instance, gatewayPort, dashboardPort)
	if err := r.Update(ctx, svc); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// createService creates a new Service for HermesAgent with both gateway and dashboard ports.
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
					Name:       "gateway",
					Port:       int32(gatewayPort),
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(gatewayPort),
				},
				{
					Name:       "dashboard",
					Port:       int32(dashboardPort),
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(dashboardPort),
				},
			},
			Selector: map[string]string{
				"hermes.io/app":        instance.Name,
				"hermes.io/managed-by": "hermes-operator",
			},
		},
	}
}

// updateService updates an existing Service.
func updateService(svc *corev1.Service, instance *corev1alpha1.HermesAgent, gatewayPort, dashboardPort int) *corev1.Service {
	svc.Spec.Selector = map[string]string{
		"hermes.io/app":        instance.Name,
		"hermes.io/managed-by": "hermes-operator",
	}

	if len(svc.Spec.Ports) != 2 {
		svc.Spec.Ports = make([]corev1.ServicePort, 2)
	}

	svc.Spec.Ports[0].Name = "gateway"
	svc.Spec.Ports[0].Port = int32(gatewayPort)
	svc.Spec.Ports[0].Protocol = corev1.ProtocolTCP
	svc.Spec.Ports[0].TargetPort = intstr.FromInt(gatewayPort)

	svc.Spec.Ports[1].Name = "dashboard"
	svc.Spec.Ports[1].Port = int32(dashboardPort)
	svc.Spec.Ports[1].Protocol = corev1.ProtocolTCP
	svc.Spec.Ports[1].TargetPort = intstr.FromInt(dashboardPort)

	return svc
}

// updateStatus updates the status of HermesAgent.
func (r *HermesAgentReconciler) updateStatus(ctx context.Context, instance *corev1alpha1.HermesAgent) error {
	deployName := getDeploymentName(instance)
	deploy := &appsv1.Deployment{}
	deployErr := r.Get(ctx, client.ObjectKey{Name: deployName, Namespace: instance.Namespace}, deploy)

	if deployErr == nil {
		instance.Status.DeploymentName = deploy.Name
		instance.Status.ReadyReplicas = deploy.Status.ReadyReplicas

		// Derive a simple phase string from Deployment conditions
		instance.Status.Phase = deploymentPhase(deploy)

		if instance.Status.StartedAt == nil {
			t := deploy.CreationTimestamp
			instance.Status.StartedAt = &t
		}
	}

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

	svcName := getServiceName(instance)
	svc := &corev1.Service{}
	svcErr := r.Get(ctx, client.ObjectKey{Name: svcName, Namespace: instance.Namespace}, svc)
	if svcErr == nil {
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
		Reason:             "DeploymentNotReady",
		Message:            "Waiting for deployment to be ready",
	}
	if deployErr == nil && deploy.Status.ReadyReplicas > 0 {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "DeploymentReady"
		readyCondition.Message = "Deployment has ready replicas"
	}

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

// deploymentPhase returns a human-readable phase string derived from Deployment status.
func deploymentPhase(deploy *appsv1.Deployment) string {
	if deploy.Status.ReadyReplicas > 0 {
		return "Running"
	}
	for _, c := range deploy.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Status == corev1.ConditionFalse {
			return "Failed"
		}
	}
	return "Pending"
}

// getDeploymentName returns the Deployment name for a HermesAgent.
func getDeploymentName(instance *corev1alpha1.HermesAgent) string {
	return fmt.Sprintf("hermes-agent-%s", instance.Name)
}

// getServiceName returns the Service name for a HermesAgent.
func getServiceName(instance *corev1alpha1.HermesAgent) string {
	return fmt.Sprintf("hermes-agent-%s", instance.Name)
}

// getPVCName returns the PVC name for a HermesAgent.
func getPVCName(instance *corev1alpha1.HermesAgent) string {
	return fmt.Sprintf("hermes-data-%s", instance.Name)
}

// getConfigMapName returns the ConfigMap name for a HermesAgent.
func getConfigMapName(instance *corev1alpha1.HermesAgent) string {
	return fmt.Sprintf("hermes-config-%s", instance.Name)
}

// containsFinalizer checks if the finalizer exists in the instance.
func containsFinalizer(instance *corev1alpha1.HermesAgent, finalizer string) bool {
	for _, f := range instance.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

// removeFinalizer removes the finalizer from the instance.
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
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Named("hermesagent").
		Complete(r)
}
