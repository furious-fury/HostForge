package main

import (
	"context"
	"fmt"
	"encoding/json"
	"errors"
	"database/sql"
	"net/http"
	"strings"

	"github.com/hostforge/hostforge/internal/git"
	"github.com/hostforge/hostforge/internal/services"
)

type projectGitAuthPutBody struct {
	Provider string `json:"provider"`
	Token    string `json:"token"`
}

func (s *server) gitAuthOptionsForProject(ctx context.Context, projectID string) (git.AuthOptions, error) {
	row, err := s.store.GetProjectGitAuthSealed(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return git.AuthOptions{}, nil
		}
		return git.AuthOptions{}, err
	}
	if s.envSealer == nil {
		return git.AuthOptions{}, fmt.Errorf("project has stored git auth but %s is not configured", "HOSTFORGE_ENV_ENCRYPTION_KEY")
	}
	if strings.ToLower(strings.TrimSpace(row.Provider)) != "github" {
		return git.AuthOptions{}, fmt.Errorf("unsupported git provider %q", row.Provider)
	}
	pt, err := s.envSealer.Open(row.TokenCT)
	if err != nil {
		return git.AuthOptions{}, err
	}
	return git.AuthOptions{GitHubToken: string(pt)}, nil
}

func (s *server) handleProjectGitAuthGet(w http.ResponseWriter, r *http.Request, projectID string) {
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
	meta, err := s.store.GetProjectGitAuthMeta(r.Context(), projectID)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusOK, map[string]any{"git_auth": apiProjectGitAuth{Configured: false, Provider: "github"}})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "git_auth_lookup_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"git_auth": apiProjectGitAuth{
		Configured: true,
		Provider:   meta.Provider,
		TokenLast4: meta.TokenLast4,
		UpdatedAt:  formatTime(meta.UpdatedAt),
	}})
}

func (s *server) handleProjectGitAuthPut(w http.ResponseWriter, r *http.Request, projectID string) {
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
	var body projectGitAuthPutBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	provider := strings.ToLower(strings.TrimSpace(body.Provider))
	if provider == "" {
		provider = "github"
	}
	if provider != "github" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "unsupported_git_provider"})
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "git_auth_token_required"})
		return
	}
	if len([]byte(token)) > 4096 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "git_auth_token_too_long"})
		return
	}
	ct, err := s.envSealer.Seal([]byte(token))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "git_auth_seal_failed"})
		return
	}
	meta, err := s.store.UpsertProjectGitHubAuth(r.Context(), projectID, ct, services.ValueLast4([]byte(token)))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "git_auth_upsert_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "git_auth": apiProjectGitAuth{
		Configured: true,
		Provider:   meta.Provider,
		TokenLast4: meta.TokenLast4,
		UpdatedAt:  formatTime(meta.UpdatedAt),
	}})
}

func (s *server) handleProjectGitAuthDelete(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodDelete {
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
	if err := s.store.DeleteProjectGitAuth(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "git_auth_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "git_auth_delete_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
