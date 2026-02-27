package broker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"

	"github.com/contextforge/contextforge-broker/config"
	"github.com/contextforge/contextforge-broker/store"
)

const (
	// OSB API version
	OSBAPIVersion = "2.17"
)

// Broker implements the Open Service Broker API
type Broker struct {
	config *config.Config
	store  store.Store
}

// New creates a new broker instance
func New(cfg *config.Config) (*Broker, error) {
	// Initialize state store
	stateStore, err := store.NewFileStore(cfg.StateStore.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to create state store: %w", err)
	}

	b := &Broker{
		config: cfg,
		store:  stateStore,
	}

	return b, nil
}

// Router returns the HTTP router for the broker
func (b *Broker) Router() http.Handler {
	r := mux.NewRouter()

	// Health check endpoint (no auth required)
	r.HandleFunc("/health", b.healthHandler).Methods("GET")

	// Icon endpoint (no auth required) - serves marketplace icon
	r.HandleFunc("/icon.png", b.iconHandler).Methods("GET")

	// OSB API endpoints
	api := r.PathPrefix("/v2").Subrouter()
	api.Use(b.authMiddleware)
	api.Use(b.osbVersionMiddleware)

	api.HandleFunc("/catalog", b.catalogHandler).Methods("GET")
	api.HandleFunc("/service_instances/{instance_id}", b.provisionHandler).Methods("PUT")
	api.HandleFunc("/service_instances/{instance_id}", b.deprovisionHandler).Methods("DELETE")
	api.HandleFunc("/service_instances/{instance_id}", b.getInstanceHandler).Methods("GET")
	api.HandleFunc("/service_instances/{instance_id}/last_operation", b.lastOperationHandler).Methods("GET")
	api.HandleFunc("/service_instances/{instance_id}/service_bindings/{binding_id}", b.bindHandler).Methods("PUT")
	api.HandleFunc("/service_instances/{instance_id}/service_bindings/{binding_id}", b.unbindHandler).Methods("DELETE")
	api.HandleFunc("/service_instances/{instance_id}/service_bindings/{binding_id}", b.getBindingHandler).Methods("GET")

	return r
}

// Middleware

func (b *Broker) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != b.config.Auth.Username || password != b.config.Auth.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="ContextForge MCP Gateway Broker"`)
			b.writeError(w, http.StatusUnauthorized, "Unauthorized", "Invalid or missing credentials")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (b *Broker) osbVersionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version := r.Header.Get("X-Broker-API-Version")
		if version == "" {
			b.writeError(w, http.StatusPreconditionFailed, "MissingAPIVersion",
				"X-Broker-API-Version header is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Handler implementations

func (b *Broker) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (b *Broker) iconHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	w.Write(iconPNG)
}

func (b *Broker) catalogHandler(w http.ResponseWriter, r *http.Request) {
	catalog := b.buildCatalog()
	b.writeJSON(w, http.StatusOK, catalog)
}

func (b *Broker) provisionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceID := vars["instance_id"]

	// Check if instance already exists
	existing, err := b.store.GetInstance(instanceID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}
	if existing != nil {
		b.writeJSON(w, http.StatusOK, map[string]any{})
		return
	}

	// Parse request
	var req ProvisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.writeError(w, http.StatusBadRequest, "BadRequest", "Invalid JSON body")
		return
	}

	// Validate service and plan
	plan := b.findPlan(req.ServiceID, req.PlanID)
	if plan == nil {
		b.writeError(w, http.StatusBadRequest, "InvalidPlan", "Unknown service or plan ID")
		return
	}

	// Create instance (synchronous, always succeeds)
	instance := &store.ServiceInstance{
		ID:               instanceID,
		ServiceID:        req.ServiceID,
		PlanID:           req.PlanID,
		OrganizationGUID: req.OrganizationGUID,
		SpaceGUID:        req.SpaceGUID,
		Parameters:       req.Parameters,
		Context:          req.Context,
		CreatedAt:        time.Now(),
		State:            "succeeded",
	}

	if err := b.store.SaveInstance(instance); err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}

	log.Printf("Provisioned instance %s (service=%s, plan=%s)", instanceID, req.ServiceID, req.PlanID)
	b.writeJSON(w, http.StatusCreated, map[string]any{})
}

func (b *Broker) deprovisionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceID := vars["instance_id"]

	instance, err := b.store.GetInstance(instanceID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}
	if instance == nil {
		b.writeError(w, http.StatusGone, "InstanceNotFound", "Service instance not found")
		return
	}

	// Check for existing bindings
	bindings, err := b.store.ListBindingsForInstance(instanceID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}
	if len(bindings) > 0 {
		b.writeError(w, http.StatusBadRequest, "BindingsExist",
			"Cannot deprovision instance with active bindings")
		return
	}

	if err := b.store.DeleteInstance(instanceID); err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}

	log.Printf("Deprovisioned instance %s", instanceID)
	b.writeJSON(w, http.StatusOK, map[string]any{})
}

func (b *Broker) getInstanceHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceID := vars["instance_id"]

	instance, err := b.store.GetInstance(instanceID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}
	if instance == nil {
		b.writeError(w, http.StatusNotFound, "InstanceNotFound", "Service instance not found")
		return
	}

	b.writeJSON(w, http.StatusOK, map[string]any{
		"service_id": instance.ServiceID,
		"plan_id":    instance.PlanID,
		"parameters": instance.Parameters,
	})
}

func (b *Broker) lastOperationHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceID := vars["instance_id"]

	instance, err := b.store.GetInstance(instanceID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}
	if instance == nil {
		b.writeError(w, http.StatusGone, "InstanceNotFound", "Service instance not found")
		return
	}

	b.writeJSON(w, http.StatusOK, map[string]any{
		"state": "succeeded",
	})
}

func (b *Broker) bindHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceID := vars["instance_id"]
	bindingID := vars["binding_id"]

	instance, err := b.store.GetInstance(instanceID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}
	if instance == nil {
		b.writeError(w, http.StatusNotFound, "InstanceNotFound", "Service instance not found")
		return
	}

	// Check if instance is ready
	if instance.State != "succeeded" {
		b.writeError(w, http.StatusUnprocessableEntity, "InstanceNotReady",
			"Service instance is not ready")
		return
	}

	// Check if binding already exists
	existing, err := b.store.GetBinding(bindingID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}
	if existing != nil {
		b.writeJSON(w, http.StatusOK, b.buildCredentials(existing))
		return
	}

	// Parse request
	var req BindRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.writeError(w, http.StatusBadRequest, "BadRequest", "Invalid JSON body")
		return
	}

	// Generate username from binding ID
	shortID := bindingID
	if len(shortID) > 16 {
		shortID = shortID[:16]
	}
	username := fmt.Sprintf("cf-binding-%s", shortID)

	// Generate JWT token
	token, err := b.generateJWT(username, bindingID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "BindError",
			fmt.Sprintf("Failed to generate JWT: %v", err))
		return
	}

	binding := &store.ServiceBinding{
		ID:         bindingID,
		InstanceID: instanceID,
		AppGUID:    req.AppGUID,
		Parameters: req.Parameters,
		CreatedAt:  time.Now(),
		Username:   username,
		JWTToken:   token,
	}

	if err := b.store.SaveBinding(binding); err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}

	log.Printf("Created binding %s for instance %s (username=%s)", bindingID, instanceID, username)
	b.writeJSON(w, http.StatusCreated, b.buildCredentials(binding))
}

func (b *Broker) unbindHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceID := vars["instance_id"]
	bindingID := vars["binding_id"]

	instance, err := b.store.GetInstance(instanceID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}
	if instance == nil {
		b.writeError(w, http.StatusGone, "InstanceNotFound", "Service instance not found")
		return
	}

	binding, err := b.store.GetBinding(bindingID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}
	if binding == nil {
		b.writeError(w, http.StatusGone, "BindingNotFound", "Service binding not found")
		return
	}

	if err := b.store.DeleteBinding(bindingID); err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}

	log.Printf("Deleted binding %s for instance %s", bindingID, instanceID)
	b.writeJSON(w, http.StatusOK, map[string]any{})
}

func (b *Broker) getBindingHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceID := vars["instance_id"]
	bindingID := vars["binding_id"]

	instance, err := b.store.GetInstance(instanceID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}
	if instance == nil {
		b.writeError(w, http.StatusNotFound, "InstanceNotFound", "Service instance not found")
		return
	}

	binding, err := b.store.GetBinding(bindingID)
	if err != nil {
		b.writeError(w, http.StatusInternalServerError, "StoreError", err.Error())
		return
	}
	if binding == nil {
		b.writeError(w, http.StatusNotFound, "BindingNotFound", "Service binding not found")
		return
	}

	b.writeJSON(w, http.StatusOK, b.buildCredentials(binding))
}

// JWT generation

func (b *Broker) generateJWT(username, bindingID string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": username,
		"iat": now.Unix(),
		"jti": bindingID,
	}

	if b.config.ContextForge.JWTExpiryHours > 0 {
		claims["exp"] = now.Add(time.Duration(b.config.ContextForge.JWTExpiryHours) * time.Hour).Unix()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(b.config.ContextForge.JWTSecretKey))
}

// Helper methods

func (b *Broker) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (b *Broker) writeError(w http.ResponseWriter, status int, errorCode, description string) {
	b.writeJSON(w, status, map[string]string{
		"error":       errorCode,
		"description": description,
	})
}

func (b *Broker) findPlan(serviceID, planID string) *config.PlanConfig {
	for _, service := range b.config.Catalog.Services {
		if service.ID == serviceID {
			for _, plan := range service.Plans {
				if plan.ID == planID {
					return &plan
				}
			}
		}
	}
	return nil
}

func (b *Broker) buildCatalog() map[string]any {
	services := make([]map[string]any, 0, len(b.config.Catalog.Services))

	for _, svc := range b.config.Catalog.Services {
		plans := make([]map[string]any, 0, len(svc.Plans))
		for _, plan := range svc.Plans {
			planData := map[string]any{
				"id":          plan.ID,
				"name":        plan.Name,
				"description": plan.Description,
				"free":        plan.Free,
				"bindable":    true,
				"metadata": map[string]any{
					"displayName": plan.Metadata.DisplayName,
					"bullets":     plan.Metadata.Bullets,
				},
			}
			plans = append(plans, planData)
		}

		// Use embedded icon as data URI if no external imageUrl configured
		imageURL := svc.Metadata.ImageURL
		if imageURL == "" && len(iconPNG) > 0 {
			imageURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(iconPNG)
		}

		serviceData := map[string]any{
			"id":                    svc.ID,
			"name":                  svc.Name,
			"description":           svc.Description,
			"bindable":              svc.Bindable,
			"instances_retrievable": true,
			"bindings_retrievable":  true,
			"plan_updateable":       false,
			"plans":                 plans,
			"tags":                  svc.Tags,
			"metadata": map[string]any{
				"displayName":         svc.Metadata.DisplayName,
				"imageUrl":            imageURL,
				"longDescription":     svc.Metadata.LongDescription,
				"providerDisplayName": svc.Metadata.ProviderDisplayName,
				"documentationUrl":    svc.Metadata.DocumentationURL,
				"supportUrl":          svc.Metadata.SupportURL,
			},
		}

		services = append(services, serviceData)
	}

	return map[string]any{
		"services": services,
	}
}

func (b *Broker) buildCredentials(binding *store.ServiceBinding) map[string]any {
	url := b.config.ContextForge.URL
	mcpURL := b.config.ContextForge.MCPURL

	// Ensure mcp_url has a value
	if mcpURL == "" && url != "" {
		mcpURL = strings.TrimRight(url, "/") + "/mcp"
	}

	return map[string]any{
		"credentials": map[string]any{
			"url":       url,
			"mcp_url":   mcpURL,
			"username":  binding.Username,
			"jwt_token": binding.JWTToken,
			"uri":       url,
		},
	}
}

// Request types

type ProvisionRequest struct {
	ServiceID        string         `json:"service_id"`
	PlanID           string         `json:"plan_id"`
	OrganizationGUID string         `json:"organization_guid"`
	SpaceGUID        string         `json:"space_guid"`
	Parameters       map[string]any `json:"parameters,omitempty"`
	Context          map[string]any `json:"context,omitempty"`
}

type BindRequest struct {
	ServiceID    string         `json:"service_id"`
	PlanID       string         `json:"plan_id"`
	AppGUID      string         `json:"app_guid,omitempty"`
	BindResource map[string]any `json:"bind_resource,omitempty"`
	Parameters   map[string]any `json:"parameters,omitempty"`
}
