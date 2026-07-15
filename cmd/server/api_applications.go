package main

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/hostforge/hostforge/internal/repository"
	platformservices "github.com/hostforge/hostforge/internal/services"
	"strings"
)

func (s *server) handleApplications(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/applications" {
		switch r.Method {
		case http.MethodGet:
			items, err := s.store.ListApplicationSummaries(r.Context())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_applications_failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"applications": items})
		case http.MethodPost:
			var req struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
				return
			}
			item, err := s.store.CreateApplication(r.Context(), req.Name, req.Description)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "create_application_failed"})
				return
			}
			_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: item.ID, EventType: "application", Status: "created", Actor: "operator", Message: "Application created", Detail: item.Name})
			writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "application": item})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/applications/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	app, err := s.store.GetApplication(r.Context(), parts[0])
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "application_not_found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "application_lookup_failed"})
		}
		return
	}
	if len(parts) == 2 && parts[1] == "services" && r.Method == http.MethodPost {
		in, ok := decodeServiceRequest(w, r, app.ID, nil)
		if !ok {
			return
		}
		item, err := s.store.CreateService(r.Context(), in)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: app.ID, ServiceID: item.ID, EventType: "service", Status: "created", Actor: "operator", Message: "Service created", Detail: item.Name})
		writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "service": item})
		return
	}
	if len(parts) == 3 && parts[1] == "environments" && r.Method == http.MethodPatch {
		environment, err := s.store.GetEnvironment(r.Context(), parts[2])
		if err != nil || environment.ApplicationID != app.ID {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "environment_not_found"})
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
			return
		}
		item, err := s.store.UpdateEnvironment(r.Context(), environment.ID, req.Name)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "update_environment_failed"})
			return
		}
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: app.ID, EnvironmentID: item.ID, EventType: "configuration", Status: "updated", Actor: "operator", Message: "Environment updated", Detail: item.Name})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "environment": item})
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			envs, envErr := s.store.ListApplicationEnvironments(r.Context(), app.ID)
			services, serviceErr := s.store.ListApplicationServices(r.Context(), app.ID)
			if envErr != nil || serviceErr != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "application_summary_failed"})
				return
			}
			bindings := make(map[string][]repository.ServiceEnvironment, len(services))
			for _, service := range services {
				items, err := s.store.ListServiceEnvironments(r.Context(), service.ID)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "application_summary_failed"})
					return
				}
				bindings[service.ID] = items
			}
			writeJSON(w, http.StatusOK, map[string]any{"application": app, "environments": envs, "services": services, "service_bindings": bindings})
		case http.MethodPatch:
			var req struct {
				Name        *string `json:"name"`
				Description *string `json:"description"`
				Archived    *bool   `json:"archived"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
				return
			}
			name, description := app.Name, app.Description
			if req.Name != nil {
				name = *req.Name
			}
			if req.Description != nil {
				description = *req.Description
			}
			item, err := s.store.UpdateApplication(r.Context(), app.ID, name, description, req.Archived)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "update_application_failed"})
				return
			}
			_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: item.ID, EventType: "application", Status: "updated", Actor: "operator", Message: "Application updated", Detail: item.Name})
			writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "application": item})
		case http.MethodDelete:
			result, err := platformservices.DeleteApplicationAndRuntime(r.Context(), s.log, s.cfg, s.store, app.ID)
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": publicAPIError(err, "delete_application_failed")})
				return
			}
			response := map[string]any{"status": "deleted"}
			if result.CaddySyncError != "" {
				response["routing_warning"] = result.CaddySyncError
			}
			writeJSON(w, http.StatusOK, response)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	if len(parts) == 2 && parts[1] == "environments" && r.Method == http.MethodGet {
		items, err := s.store.ListApplicationEnvironments(r.Context(), app.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_environments_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"environments": items})
		return
	}
	if len(parts) == 2 && parts[1] == "environments" && r.Method == http.MethodPost {
		var req struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
			Kind string `json:"kind"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
			return
		}
		item, err := s.store.CreateEnvironment(r.Context(), app.ID, req.Name, req.Slug, req.Kind)
		if err != nil {
			status, code := http.StatusBadRequest, "create_environment_failed"
			if err == repository.ErrDuplicateEnvironment {
				status, code = http.StatusConflict, "duplicate_environment"
			}
			writeJSON(w, status, map[string]string{"status": "error", "error": code})
			return
		}
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: app.ID, EnvironmentID: item.ID, EventType: "configuration", Status: "created", Actor: "operator", Message: "Environment created", Detail: item.Name})
		writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "environment": item})
		return
	}
	if len(parts) == 2 && parts[1] == "services" && r.Method == http.MethodGet {
		items, err := s.store.ListApplicationServices(r.Context(), app.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_services_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"services": items})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
}
