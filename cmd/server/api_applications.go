package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

func (s *server) handleApplications(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/applications" {
		switch r.Method {
		case http.MethodGet:
			items, err := s.store.ListApplications(r.Context())
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
			writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "application": item})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/applications/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
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
	if len(parts) == 1 && r.Method == http.MethodGet {
		envs, _ := s.store.ListApplicationEnvironments(r.Context(), app.ID)
		services, _ := s.store.ListApplicationServices(r.Context(), app.ID)
		writeJSON(w, http.StatusOK, map[string]any{"application": app, "environments": envs, "services": services})
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
	if len(parts) == 2 && parts[1] == "services" && r.Method == http.MethodGet {
		items, err := s.store.ListApplicationServices(r.Context(), app.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_services_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"services": items})
		return
	}
	http.NotFound(w, r)
}
