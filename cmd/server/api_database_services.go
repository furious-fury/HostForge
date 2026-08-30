package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/repository"
	platformservices "github.com/furious-fury/HostForge/internal/services"
)

func (s *server) handleDatabaseServices(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/database-services/"), "/"), "/")
	if len(parts) == 1 && parts[0] != "" && r.Method == http.MethodGet {
		proxied := r.Clone(r.Context())
		proxiedURL := *r.URL
		proxiedURL.Path = "/api/services/" + parts[0]
		proxied.URL = &proxiedURL
		s.handleServices(w, proxied)
		return
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "purge" && r.Method == http.MethodDelete {
		s.handlePurgeDatabaseService(w, r, parts[0])
		return
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] != "restore-deleted" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	service, err := s.store.GetService(r.Context(), parts[0])
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_service_not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "database_service_lookup_failed"})
		return
	}
	if service.ServiceType != "database" {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "service_type_not_database"})
		return
	}
	operations, err := s.store.RestoreDeletedDatabaseService(r.Context(), service.ID, "operator")
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_restore_unavailable"})
		return
	}
	_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{
		ApplicationID: service.ApplicationID, ServiceID: service.ID,
		EventType: "database", Status: "queued", Actor: "operator",
		Message: "Database restore queued", Detail: service.Name,
	})
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "operations": operations})
}

func (s *server) handlePurgeDatabaseService(w http.ResponseWriter, r *http.Request, serviceID string) {
	service, err := s.store.GetService(r.Context(), serviceID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_service_not_found"})
		return
	}
	if err != nil || service.ServiceType != "database" {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "service_type_not_database"})
		return
	}
	var req struct {
		Confirmation string `json:"confirmation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	if strings.TrimSpace(req.Confirmation) != service.Name {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "database_purge_confirmation_mismatch"})
		return
	}
	if err := platformservices.PurgeDatabaseServiceAndRuntime(r.Context(), s.log, s.store, service.ID, time.Now().UTC(), "operator"); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": publicAPIError(err, "database_purge_unavailable")})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "purged"})
}
