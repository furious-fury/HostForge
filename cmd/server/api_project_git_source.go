package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hostforge/hostforge/internal/models"
)

// projectGitSourcePutBody lets the UI switch a project between url / github_app / ssh.
// When setting github_app, installation_id is required.
type projectGitSourcePutBody struct {
	GitSource            string `json:"git_source"`
	GitHubInstallationID int64  `json:"github_installation_id,omitempty"`
}

// handleProjectGitSourcePut updates the credential mode for one project.
func (s *server) handleProjectGitSourcePut(w http.ResponseWriter, r *http.Request, projectID string) {
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"status": "error", "error": "content_type_must_be_application_json"})
		return
	}
	defer r.Body.Close()
	var body projectGitSourcePutBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	gs := strings.TrimSpace(body.GitSource)
	switch gs {
	case models.GitSourceURL, models.GitSourceGitHubApp, models.GitSourceSSH:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_git_source"})
		return
	}
	installationID := body.GitHubInstallationID
	if gs == models.GitSourceGitHubApp {
		if installationID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "installation_id_required"})
			return
		}
		if _, err := s.store.GetGitHubInstallation(r.Context(), installationID); err != nil {
			if errorsIsNoRows(err) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "installation_not_found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "installation_lookup_failed"})
			return
		}
	} else {
		installationID = 0
	}
	if err := s.store.UpdateProjectGitSource(r.Context(), projectID, gs, installationID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "git_source_update_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":                 "ok",
		"git_source":             gs,
		"github_installation_id": installationID,
	})
}
