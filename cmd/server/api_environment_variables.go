package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/furious-fury/HostForge/internal/services"
)

type environmentVariableRequest struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	ServiceID string `json:"service_id"`
}

func (s *server) validateVariableScope(w http.ResponseWriter, r *http.Request, applicationID, environmentID, serviceID string) bool {
	environment, err := s.store.GetEnvironment(r.Context(), environmentID)
	if err != nil || environment.ApplicationID != applicationID {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "environment_not_found"})
		return false
	}
	if strings.TrimSpace(serviceID) == "" {
		return true
	}
	service, err := s.store.GetService(r.Context(), serviceID)
	if err != nil || service.ApplicationID != applicationID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_service_scope"})
		return false
	}
	if _, err := s.store.GetServiceEnvironment(r.Context(), service.ID, environmentID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "service_environment_not_found"})
		return false
	}
	return true
}

func (s *server) handleEnvironmentVariables(w http.ResponseWriter, r *http.Request, applicationID, environmentID, variableID string) {
	if !s.requireEnvSealer(w) {
		return
	}
	if variableID == "" {
		switch r.Method {
		case http.MethodGet:
			serviceID := strings.TrimSpace(r.URL.Query().Get("service_id"))
			if !s.validateVariableScope(w, r, applicationID, environmentID, serviceID) {
				return
			}
			items, err := s.store.ListEnvironmentVariableMeta(r.Context(), applicationID, environmentID, serviceID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_variables_failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"variables": items})
		case http.MethodPost:
			var req environmentVariableRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
				return
			}
			if !s.validateVariableScope(w, r, applicationID, environmentID, req.ServiceID) {
				return
			}
			key, code := services.ValidateEnvEntry(req.Key, req.Value)
			if code != "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": code})
				return
			}
			ciphertext, err := s.envSealer.Seal([]byte(req.Value))
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "env_seal_failed"})
				return
			}
			item, err := s.store.UpsertEnvironmentVariable(r.Context(), applicationID, environmentID, req.ServiceID, key, ciphertext, services.ValueLast4([]byte(req.Value)))
			if errors.Is(err, repository.ErrEnvironmentVariableLimitExceeded) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "env_too_many_keys"})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "upsert_variable_failed"})
				return
			}
			_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: applicationID, ServiceID: item.ServiceID, EnvironmentID: environmentID, EventType: "configuration", Status: "updated", Actor: "operator", Message: "Environment variable replaced", Detail: item.Key})
			writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "variable": item})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
			return
		}
		if len([]byte(req.Value)) > services.MaxEnvValueLen {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "env_value_too_long"})
			return
		}
		ciphertext, err := s.envSealer.Seal([]byte(req.Value))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "env_seal_failed"})
			return
		}
		item, err := s.store.UpdateEnvironmentVariableValue(r.Context(), applicationID, environmentID, variableID, ciphertext, services.ValueLast4([]byte(req.Value)))
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "variable_not_found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "update_variable_failed"})
			return
		}
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: applicationID, ServiceID: item.ServiceID, EnvironmentID: environmentID, EventType: "configuration", Status: "updated", Actor: "operator", Message: "Environment variable replaced", Detail: item.Key})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "variable": item})
	case http.MethodDelete:
		err := s.store.DeleteEnvironmentVariable(r.Context(), applicationID, environmentID, variableID)
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "variable_not_found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "delete_variable_failed"})
			return
		}
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: applicationID, EnvironmentID: environmentID, EventType: "configuration", Status: "deleted", Actor: "operator", Message: "Environment variable deleted", Detail: variableID})
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
	}
}
