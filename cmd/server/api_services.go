package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/hostforge/hostforge/internal/repository"
	platformservices "github.com/hostforge/hostforge/internal/services"
)

type serviceRequest struct {
	Name                 string `json:"name"`
	RepoURL              string `json:"repo_url"`
	GitHubInstallationID int64  `json:"github_installation_id"`
	RootDirectory        string `json:"root_directory"`
	Runtime              string `json:"runtime"`
	InstallCmd           string `json:"install_cmd"`
	BuildCmd             string `json:"build_cmd"`
	StartCmd             string `json:"start_cmd"`
	InternalPort         int    `json:"internal_port"`
	HealthCheckPath      string `json:"health_check_path"`
}

func decodeServiceRequest(w http.ResponseWriter, r *http.Request, applicationID string, current *repository.Service) (repository.CreateServiceInput, bool) {
	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return repository.CreateServiceInput{}, false
	}
	var req serviceRequest
	var present map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil || json.Unmarshal(body, &present) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return repository.CreateServiceInput{}, false
	}
	if current != nil {
		if _, ok := present["name"]; !ok {
			req.Name = current.Name
		}
		if _, ok := present["repo_url"]; !ok {
			req.RepoURL = current.RepoURL
		}
		if _, ok := present["runtime"]; !ok {
			req.Runtime = current.DeployRuntime
		}
		if _, ok := present["internal_port"]; !ok {
			req.InternalPort = current.InternalPort
		}
		if _, ok := present["health_check_path"]; !ok {
			req.HealthCheckPath = current.HealthCheckPath
		}
		if _, ok := present["github_installation_id"]; !ok {
			req.GitHubInstallationID = current.GitHubInstallationID
		}
		if _, ok := present["root_directory"]; !ok {
			req.RootDirectory = current.RootDirectory
		}
		if _, ok := present["install_cmd"]; !ok {
			req.InstallCmd = current.InstallCmd
		}
		if _, ok := present["build_cmd"]; !ok {
			req.BuildCmd = current.BuildCmd
		}
		if _, ok := present["start_cmd"]; !ok {
			req.StartCmd = current.StartCmd
		}
	}
	repoURL, err := platformservices.CanonicalRepoURL(req.RepoURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_repository_clone_url"})
		return repository.CreateServiceInput{}, false
	}
	runtime, install, build, start, code := platformservices.ValidateDeployFields(req.Runtime, req.InstallCmd, req.BuildCmd, req.StartCmd)
	if code != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": code})
		return repository.CreateServiceInput{}, false
	}
	if req.InternalPort < 1 || req.InternalPort > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_internal_port"})
		return repository.CreateServiceInput{}, false
	}
	return repository.CreateServiceInput{ApplicationID: applicationID, Name: req.Name, RepoURL: repoURL, GitHubInstallationID: req.GitHubInstallationID, RootDirectory: req.RootDirectory, DeployRuntime: runtime, InstallCmd: install, BuildCmd: build, StartCmd: start, InternalPort: req.InternalPort, HealthCheckPath: req.HealthCheckPath}, true
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrServiceNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "service_not_found"})
	case errors.Is(err, repository.ErrApplicationNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "application_not_found"})
	case errors.Is(err, repository.ErrDuplicateService):
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "duplicate_service"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "service_operation_failed"})
	}
}

func (s *server) handleServices(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/services/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	service, err := s.store.GetService(r.Context(), parts[0])
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]any{"service": service})
		case http.MethodPatch:
			in, ok := decodeServiceRequest(w, r, service.ApplicationID, &service)
			if !ok {
				return
			}
			item, err := s.store.UpdateService(r.Context(), service.ID, in)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": item})
		case http.MethodDelete:
			if err := s.store.DeleteService(r.Context(), service.ID); err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	if len(parts) == 3 && parts[1] == "environments" {
		environment, err := s.store.GetEnvironment(r.Context(), parts[2])
		if err != nil || environment.ApplicationID != service.ApplicationID {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "environment_not_found"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			item, err := s.store.GetServiceEnvironment(r.Context(), service.ID, environment.ID)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"binding": item})
		case http.MethodPatch:
			var req struct {
				Branch     string `json:"branch"`
				AutoDeploy bool   `json:"auto_deploy"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
				return
			}
			if req.Branch != "" {
				if _, err := s.store.GetApplication(r.Context(), service.ApplicationID); err != nil {
					writeServiceError(w, err)
					return
				}
			}
			item, err := s.store.UpdateServiceEnvironment(r.Context(), service.ID, environment.ID, req.Branch, req.AutoDeploy)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "binding": item})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	http.NotFound(w, r)
}
