package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/hermes-operator/hermes-operator/api/v1"
)

const (
	apiGroup    = "/api/v1"
	contentType = "application/json"
)

// Server represents the REST API server
type Server struct {
	client client.Client
	scheme *runtime.Scheme
	mux    *http.ServeMux
	port   int
	log    logr.Logger
}

// HermesAgentRequest represents the request body for creating a HermesAgent
type HermesAgentRequest struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace,omitempty"`
	Model        string            `json:"model"`
	Provider     string            `json:"provider"`
	BaseURL      string            `json:"baseURL,omitempty"`
	APISecretRef SecretRefRequest  `json:"apiSecretRef"`
	MaxTurns     int               `json:"maxTurns,omitempty"`
	Personality  string            `json:"personality,omitempty"`
	Image        string            `json:"image,omitempty"`
	ServicePort  int               `json:"servicePort,omitempty"`
}

// SecretRefRequest represents a reference to a Kubernetes Secret in API requests
type SecretRefRequest struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Key       string `json:"key,omitempty"`
}

// HermesAgentResponse represents the response for a HermesAgent
type HermesAgentResponse struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Model             string            `json:"model"`
	Provider          string            `json:"provider"`
	BaseURL           string            `json:"baseURL,omitempty"`
	MaxTurns          int               `json:"maxTurns,omitempty"`
	Personality       string            `json:"personality,omitempty"`
	Image             string            `json:"image,omitempty"`
	ServicePort       int               `json:"servicePort,omitempty"`
	Phase             string            `json:"phase,omitempty"`
	Endpoint          string            `json:"endpoint,omitempty"`
	PodIP             string            `json:"podIP,omitempty"`
	ServiceName       string            `json:"serviceName,omitempty"`
	Conditions        []Condition       `json:"conditions,omitempty"`
	CreationTimestamp metav1.Time       `json:"creationTimestamp,omitempty"`
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
	if req.Model == "" {
		s.respondError(w, http.StatusBadRequest, "Model is required")
		return
	}
	if req.Provider == "" {
		s.respondError(w, http.StatusBadRequest, "Provider is required")
		return
	}
	if req.APISecretRef.Name == "" {
		s.respondError(w, http.StatusBadRequest, "APISecretRef.name is required")
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
		},
		Spec: corev1alpha1.HermesAgentSpec{
			Model:    req.Model,
			Provider: req.Provider,
			BaseURL:  req.BaseURL,
			APISecretRef: corev1alpha1.SecretRef{
				Name:      req.APISecretRef.Name,
				Namespace: req.APISecretRef.Namespace,
				Key:       req.APISecretRef.Key,
			},
			MaxTurns:    req.MaxTurns,
			Personality: req.Personality,
			Image:       req.Image,
			ServicePort: req.ServicePort,
		},
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
	if req.Model != "" {
		instance.Spec.Model = req.Model
	}
	if req.Provider != "" {
		instance.Spec.Provider = req.Provider
	}
	if req.BaseURL != "" {
		instance.Spec.BaseURL = req.BaseURL
	}
	if req.APISecretRef.Name != "" {
		instance.Spec.APISecretRef.Name = req.APISecretRef.Name
		if req.APISecretRef.Namespace != "" {
			instance.Spec.APISecretRef.Namespace = req.APISecretRef.Namespace
		}
		if req.APISecretRef.Key != "" {
			instance.Spec.APISecretRef.Key = req.APISecretRef.Key
		}
	}
	if req.MaxTurns != 0 {
		instance.Spec.MaxTurns = req.MaxTurns
	}
	if req.Personality != "" {
		instance.Spec.Personality = req.Personality
	}
	if req.Image != "" {
		instance.Spec.Image = req.Image
	}
	if req.ServicePort != 0 {
		instance.Spec.ServicePort = req.ServicePort
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
		Model:             instance.Spec.Model,
		Provider:          instance.Spec.Provider,
		BaseURL:           instance.Spec.BaseURL,
		MaxTurns:          instance.Spec.MaxTurns,
		Personality:       instance.Spec.Personality,
		Image:             instance.Spec.Image,
		ServicePort:       instance.Spec.ServicePort,
		Phase:             instance.Status.Phase,
		Endpoint:          instance.Status.Endpoint,
		PodIP:             instance.Status.PodIP,
		ServiceName:       instance.Status.ServiceName,
		CreationTimestamp: instance.CreationTimestamp,
		Conditions:        make([]Condition, 0),
	}

	if instance.Spec.Image == "" {
		resp.Image = "ghcr.io/aisuko/hermes:latest"
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
