package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/hostforge/hostforge/internal/crypto/envcrypt"
	"github.com/hostforge/hostforge/internal/models"
	"github.com/hostforge/hostforge/internal/repository"
	"github.com/hostforge/hostforge/internal/services"
)

func (s *server) requireEnvSealer(w http.ResponseWriter) bool {
	if s.envSealer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "env_encryption_key_missing"})
		return false
	}
	return true
}

func envVarToAPI(m models.ProjectEnvVar) apiProjectEnvVar {
	return apiProjectEnvVar{
		ID:         m.ID,
		Key:        m.Key,
		ValueLast4: m.ValueLast4,
		UpdatedAt:  formatTime(m.UpdatedAt),
	}
}

type projectEnvPair struct {
	Key   string
	Value string
}

func filterProjectEnvPairs(in []struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}) []projectEnvPair {
	var out []projectEnvPair
	for _, e := range in {
		if strings.TrimSpace(e.Key) == "" && strings.TrimSpace(e.Value) == "" {
			continue
		}
		out = append(out, projectEnvPair{Key: e.Key, Value: e.Value})
	}
	return out
}

func sealProjectEnvBatch(sealer *envcrypt.Sealer, rows []projectEnvPair) ([]repository.SealedEnvVar, string) {
	if len(rows) > services.MaxEnvVarsPerProject {
		return nil, "env_too_many_keys"
	}
	seen := make(map[string]struct{})
	out := make([]repository.SealedEnvVar, 0, len(rows))
	for _, row := range rows {
		ek, code := services.ValidateEnvEntry(row.Key, row.Value)
		if code != "" {
			return nil, code
		}
		if _, ok := seen[ek]; ok {
			return nil, "env_duplicate_key"
		}
		seen[ek] = struct{}{}
		ct, err := sealer.Seal([]byte(row.Value))
		if err != nil {
			return nil, "env_seal_failed"
		}
		out = append(out, repository.SealedEnvVar{
			Key:        ek,
			ValueCT:    ct,
			ValueLast4: services.ValueLast4([]byte(row.Value)),
		})
	}
	return out, ""
}

func (s *server) handleProjectEnvList(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	if !s.requireEnvSealer(w) {
		return
	}
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	items, err := s.store.ListProjectEnvMeta(r.Context(), projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_env_failed"})
		return
	}
	out := make([]apiProjectEnvVar, 0, len(items))
	for _, m := range items {
		out = append(out, envVarToAPI(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"env_vars": out})
}

type projectEnvKeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (s *server) handleProjectEnvPost(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	if !s.requireEnvSealer(w) {
		return
	}
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	if !strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"status": "error", "error": "content_type_must_be_application_json"})
		return
	}
	defer r.Body.Close()
	var body projectEnvKeyValue
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	ek, code := services.ValidateEnvEntry(body.Key, body.Value)
	if code != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": code})
		return
	}
	ct, err := s.envSealer.Seal([]byte(body.Value))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "env_seal_failed"})
		return
	}
	last4 := services.ValueLast4([]byte(body.Value))
	rec, err := s.store.UpsertProjectEnvVar(r.Context(), projectID, ek, ct, last4)
	if err != nil {
		if errors.Is(err, repository.ErrProjectEnvLimitExceeded) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "env_too_many_keys"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "upsert_env_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "env_var": envVarToAPI(rec)})
}

type projectEnvValueBody struct {
	Value string `json:"value"`
}

func (s *server) handleProjectEnvPut(w http.ResponseWriter, r *http.Request, projectID, envID string) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	if !s.requireEnvSealer(w) {
		return
	}
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	if !strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"status": "error", "error": "content_type_must_be_application_json"})
		return
	}
	defer r.Body.Close()
	var body projectEnvValueBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	if len([]byte(body.Value)) > services.MaxEnvValueLen {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "env_value_too_long"})
		return
	}
	if _, err := s.store.GetProjectEnvMetaByID(r.Context(), projectID, envID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "env_var_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "env_lookup_failed"})
		return
	}
	ct, err := s.envSealer.Seal([]byte(body.Value))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "env_seal_failed"})
		return
	}
	last4 := services.ValueLast4([]byte(body.Value))
	rec, err := s.store.UpdateProjectEnvValue(r.Context(), projectID, envID, ct, last4)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "env_var_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "update_env_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "env_var": envVarToAPI(rec)})
}

func (s *server) handleProjectEnvDelete(w http.ResponseWriter, r *http.Request, projectID, envID string) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	if !s.requireEnvSealer(w) {
		return
	}
	if err := s.store.DeleteProjectEnvVar(r.Context(), projectID, envID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "env_var_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "delete_env_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
