package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/hermes-operator/hermes-operator/api/v1"
)

const (
	apiGroup   = "/api/v1"
	contentType = "application/json"
)

// Server represents the REST API server
type Server struct {
	client    client.Client
	scheme    *runtime.Scheme
	mux       *http.ServeMux
	port      int
	log       logr.Logger
}

// HermesAgentRequest represents the request body for creating a HermesAgent
type HermesAgentRequest struct {
	Name                 string            `json:"name"`
	Namespace            string            `json:"namespace,omitempty"`
	Image                string            `json:"image,omitempty"`
	ImagePullPolicy      string            `json:"imagePullPolicy,omitempty"`
	ServicePort          int32             `json:"servicePort,omitempty"`
	Config               map[string]string `json:"config,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
	Annotations          map[string]string `json:"annotations,omitempty"`
	Resources            *corev1.ResourceRequirements `json:"resources,omitempty"`
	Model                string            `json:"model,omitempty"`
	APIKeySecretRef      *SecretRef        `json:"apiKeySecretRef,omitempty"`
	Tools                []string          `json:"tools,omitempty"`
	MaxIterations        int32             `json:"maxIterations,omitempty"`
	SystemPrompt         string            `json:"systemPrompt,omitempty"`
}

// SecretRef represents a reference to a Kubernetes Secret
type SecretRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// HermesAgentResponse represents the response for a HermesAgent
type HermesAgentResponse struct {
	Name                 string            `json:"name"`
	Namespace            string            `json:"namespace"`
	Image                string            `json:"image,omitempty"`
	ServicePort          int32             `json:"servicePort"`
	Phase                string            `json:"phase,omitempty"`
	Endpoint             string            `json:"endpoint,omitempty"`
	PodIP                string            `json:"podIP,omitempty"`
	ReadyReplicas        int32             `json:"readyReplicas,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
	Annotations          map[string]string `json:"annotations,omitempty"`
	Conditions           []Condition       `json:"conditions,omitempty"`
	CreationTimestamp    metav1.Time       `json:"creationTimestamp,omitempty"`
}

// Condition represents a condition of the HermesAgent
type Condition struct {
	Type               string      `json:"type"`
	Status             string      `json:"status"`
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	Reason             string      `json:"reason,omitempty"`
	Message            string      `json:"message,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// ListResponse represents a list response
type ListResponse struct {
	Items      []HermesAgentResponse `json:"items"`
	TotalCount int                   `json:"totalCount"`
}

// NewServer creates a new REST API server
func NewServer(c client.Client, s *runtime.Scheme, port int) *Server {
	return &Server{
		client: c,
		scheme: s,
		mux:    http.NewServeMux(),
		port:   port,
		log:    log.Log.WithName("api-server"),
	}
}

// RegisterRoutes registers the API routes
func (s *Server) RegisterRoutes() {
	// HermesAgent routes
	s.mux.HandleFunc(fmt.Sprintf("%s/namespaces/", apiGroup), s.handleNamespaces())
	s.mux.HandleFunc(fmt.Sprintf("%s/hermesagents", apiGroup), s.handleHermesAgents())
	s.mux.HandleFunc(fmt.Sprintf("%s/hermesagent/", apiGroup), s.handleHermesAgent())

	// Health check
	s.mux.HandleFunc("/healthz", s.handleHealthz())

	// API documentation
	s.mux.HandleFunc("/", s.handleRoot())
}

// Start starts the REST API server
func (s *Server) Start(ctx context.Context) error {
	s.RegisterRoutes()
	addr := fmt.Sprintf(":%d", s.port)
	server := &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}

	s.log.Info("Starting REST API server", "address", addr)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error(err, "REST API server error")
		}
	}()

	<-ctx.Done()
	return server.Shutdown(context.Background())
}

// handleHealthz handles health check requests
func (s *Server) handleHealthz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	}
}

// handleRoot handles root path requests
func (s *Server) handleRoot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"name":"Hermes Operator API","version":"v1","endpoints":["/api/v1/hermesagents","/api/v1/hermesagent/{name}"]}`)
	}
}

// handleNamespaces handles namespace-scoped requests
func (s *Server) handleNamespaces() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract namespace from path
		path := r.URL.Path
		namespace := extractNamespace(path)

		if namespace == "" {
			s.respondError(w, http.StatusBadRequest, "Invalid namespace path")
			return
		}

		switch r.Method {
		case http.MethodGet:
			s.listHermesAgentsByNamespace(w, r, namespace)
		default:
			s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}
}

// handleHermesAgents handles /api/v1/hermesagents
func (s *Server) handleHermesAgents() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.listHermesAgents(w, r)
		case http.MethodPost:
			s.createHermesAgent(w, r)
		default:
			s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}
}

// handleHermesAgent handles /api/v1/hermesagent/{name}
func (s *Server) handleHermesAgent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract name from path
		path := r.URL.Path
		name := extractName(path)

		if name == "" {
			s.respondError(w, http.StatusBadRequest, "HermesAgent name is required")
			return
		}

		// Extract namespace from query or path
		namespace := r.URL.Query().Get("namespace")
		if namespace == "" {
			namespace = "default"
		}

		switch r.Method {
		case http.MethodGet:
			s.getHermesAgent(w, r, namespace, name)
		case http.MethodDelete:
			s.deleteHermesAgent(w, r, namespace, name)
		case http.MethodPatch:
			s.patchHermesAgent(w, r, namespace, name)
		default:
			s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}
}

// listHermesAgents lists all HermesAgents
func (s *Server) listHermesAgents(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	namespace := r.URL.Query().Get("namespace")

	var list *corev1alpha1.HermesAgentList
	if namespace != "" {
		list = &corev1alpha1.HermesAgentList{}
		err := s.client.List(ctx, list, client.InNamespace(namespace))
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		list = &corev1alpha1.HermesAgentList{}
		err := s.client.List(ctx, list)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	response := ListResponse{
		Items:      make([]HermesAgentResponse, 0, len(list.Items)),
		TotalCount: len(list.Items),
	}

	for _, item := range list.Items {
		response.Items = append(response.Items, s.toResponse(&item))
	}

	s.respondJSON(w, http.StatusOK, response)
}

// listHermesAgentsByNamespace lists HermesAgents in a specific namespace
func (s *Server) listHermesAgentsByNamespace(w http.ResponseWriter, r *http.Request, namespace string) {
	ctx := context.Background()
	list := &corev1alpha1.HermesAgentList{}
	err := s.client.List(ctx, list, client.InNamespace(namespace))
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := ListResponse{
		Items:      make([]HermesAgentResponse, 0, len(list.Items)),
		TotalCount: len(list.Items),
	}

	for _, item := range list.Items {
		response.Items = append(response.Items, s.toResponse(&item))
	}

	s.respondJSON(w, http.StatusOK, response)
}

// getHermesAgent gets a HermesAgent by name
func (s *Server) getHermesAgent(w http.ResponseWriter, r *http.Request, namespace, name string) {
	ctx := context.Background()
	instance := &corev1alpha1.HermesAgent{}
	err := s.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			s.respondError(w, http.StatusNotFound, fmt.Sprintf("HermesAgent %s/%s not found", namespace, name))
			return
		}
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, s.toResponse(instance))
}

// createHermesAgent creates a new HermesAgent
func (s *Server) createHermesAgent(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	var req HermesAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %s", err.Error()))
		return
	}

	// Validate required fields
	if req.Name == "" {
		s.respondError(w, http.StatusBadRequest, "Name is required")
		return
	}

	namespace := req.Namespace
	if namespace == "" {
		namespace = "default"
	}

	// Check if already exists
	existing := &corev1alpha1.HermesAgent{}
	err := s.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: req.Name}, existing)
	if err == nil {
		s.respondError(w, http.StatusConflict, fmt.Sprintf("HermesAgent %s/%s already exists", namespace, req.Name))
		return
	}
	if !errors.IsNotFound(err) {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Create HermesAgent
	instance := &corev1alpha1.HermesAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: namespace,
			Labels:    req.Labels,
			Annotations: req.Annotations,
		},
		Spec: corev1alpha1.HermesAgentSpec{
			Image:           req.Image,
			ServicePort:     req.ServicePort,
			Config:          req.Config,
			Labels:          req.Labels,
			Annotations:     req.Annotations,
			Resources:       corev1.ResourceRequirements{},
		},
	}

	// Set image pull policy
	if req.ImagePullPolicy != "" {
		instance.Spec.ImagePullPolicy = corev1.PullPolicy(req.ImagePullPolicy)
	}

	// Set resources
	if req.Resources != nil {
		instance.Spec.Resources = *req.Resources
	}

	// Set Hermes config
	if req.Model != "" || len(req.Tools) > 0 || req.APIKeySecretRef != nil {
		instance.Spec.HermesConfig = &corev1alpha1.HermesConfigSpec{
			Model:          req.Model,
			Tools:          req.Tools,
			MaxIterations:  req.MaxIterations,
			SystemPrompt:   req.SystemPrompt,
		}
		if req.APIKeySecretRef != nil {
			instance.Spec.HermesConfig.APIKeySecretRef = &corev1alpha1.SecretRef{
				Name:      req.APIKeySecretRef.Name,
				Namespace: req.APIKeySecretRef.Namespace,
			}
		}
	}

	if err := s.client.Create(ctx, instance); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch the created instance to get the full status
	s.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: req.Name}, instance)

	s.respondJSON(w, http.StatusCreated, s.toResponse(instance))
}

// deleteHermesAgent deletes a HermesAgent
func (s *Server) deleteHermesAgent(w http.ResponseWriter, r *http.Request, namespace, name string) {
	ctx := context.Background()
	instance := &corev1alpha1.HermesAgent{}
	err := s.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			s.respondError(w, http.StatusNotFound, fmt.Sprintf("HermesAgent %s/%s not found", namespace, name))
			return
		}
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.client.Delete(ctx, instance); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// patchHermesAgent patches a HermesAgent
func (s *Server) patchHermesAgent(w http.ResponseWriter, r *http.Request, namespace, name string) {
	ctx := context.Background()
	instance := &corev1alpha1.HermesAgent{}
	err := s.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			s.respondError(w, http.StatusNotFound, fmt.Sprintf("HermesAgent %s/%s not found", namespace, name))
			return
		}
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req HermesAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %s", err.Error()))
		return
	}

	// Update spec
	if req.Image != "" {
		instance.Spec.Image = req.Image
	}
	if req.ImagePullPolicy != "" {
		instance.Spec.ImagePullPolicy = corev1.PullPolicy(req.ImagePullPolicy)
	}
	if req.ServicePort != 0 {
		instance.Spec.ServicePort = req.ServicePort
	}
	if req.Config != nil {
		instance.Spec.Config = req.Config
	}
	if req.Labels != nil {
		instance.Labels = req.Labels
		instance.Spec.Labels = req.Labels
	}
	if req.Annotations != nil {
		instance.Annotations = req.Annotations
		instance.Spec.Annotations = req.Annotations
	}
	if req.Resources != nil {
		instance.Spec.Resources = *req.Resources
	}

	if err := s.client.Update(ctx, instance); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, s.toResponse(instance))
}

// toResponse converts a HermesAgent to a response
func (s *Server) toResponse(instance *corev1alpha1.HermesAgent) HermesAgentResponse {
	resp := HermesAgentResponse{
		Name:              instance.Name,
		Namespace:         instance.Namespace,
		Image:             instance.Spec.Image,
		ServicePort:       instance.Spec.ServicePort,
		Phase:             instance.Status.Phase,
		Endpoint:          instance.Status.Endpoint,
		PodIP:             instance.Status.PodIP,
		ReadyReplicas:     instance.Status.ReadyReplicas,
		Labels:            instance.Labels,
		Annotations:       instance.Annotations,
		CreationTimestamp: instance.CreationTimestamp,
		Conditions:        make([]Condition, 0),
	}

	if instance.Spec.Image == "" {
		resp.Image = "ghcr.io/hermes-project/hermes-agent:latest"
	}

	for _, cond := range instance.Status.Conditions {
		resp.Conditions = append(resp.Conditions, Condition{
			Type:               cond.Type,
			Status:             string(cond.Status),
			LastTransitionTime: cond.LastTransitionTime,
			Reason:             cond.Reason,
			Message:            cond.Message,
		})
	}

	return resp
}

// respondJSON responds with JSON
func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.log.Error(err, "Failed to encode response")
	}
}

// respondError responds with an error
func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}

// extractNamespace extracts namespace from path like /api/v1/namespaces/default/...
func extractNamespace(path string) string {
	prefix := "/api/v1/namespaces/"
	if len(path) > len(prefix) {
		remaining := path[len(prefix):]
		for i, c := range remaining {
			if c == '/' {
				return remaining[:i]
			}
		}
		return remaining
	}
	return ""
}

// extractName extracts name from path like /api/v1/hermesagent/test-agent
func extractName(path string) string {
	prefix := "/api/v1/hermesagent/"
	if len(path) > len(prefix) {
		name := path[len(prefix):]
		// Remove query string if present
		for i, c := range name {
			if c == '?' {
				return name[:i]
			}
		}
		return name
	}
	return ""
}
