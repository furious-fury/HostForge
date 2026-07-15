package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/hostforge/hostforge/internal/repository"
	platformservices "github.com/hostforge/hostforge/internal/services"
)

func (s *server) serviceEnvironmentStates(r *http.Request, service repository.Service, bindings []repository.ServiceEnvironment) []map[string]any {
	states := make([]map[string]any, 0, len(bindings))
	for _, binding := range bindings {
		state := map[string]any{
			"environment_id": binding.EnvironmentID, "branch": binding.Branch, "auto_deploy": binding.AutoDeploy,
			"desired_state": binding.DesiredState, "active_deployment_id": binding.ActiveDeploymentID,
		}
		if environment, err := s.store.GetEnvironment(r.Context(), binding.EnvironmentID); err == nil {
			state["environment_name"] = environment.Name
			state["environment_kind"] = environment.Kind
		}
		if binding.ActiveDeploymentID != "" {
			if deployment, err := s.store.GetServiceDeployment(r.Context(), binding.ActiveDeploymentID); err == nil {
				state["active_deployment"] = deploymentToV2(deployment)
			}
			if container, err := s.store.GetContainerByDeploymentID(r.Context(), binding.ActiveDeploymentID); err == nil {
				state["current_container"] = map[string]any{"id": container.ID, "docker_container_id": container.DockerContainerID, "internal_port": container.InternalPort, "host_port": container.HostPort, "status": container.Status, "updated_at": container.UpdatedAt}
			} else if !errors.Is(err, sql.ErrNoRows) {
				state["container_error"] = "container_lookup_failed"
			}
		}
		if domains, err := s.store.ListServiceDomains(r.Context(), service.ApplicationID, binding.EnvironmentID, service.ID); err == nil && len(domains) > 0 {
			state["public_url"] = "https://" + domains[0].DomainName
			state["domains"] = domains
		}
		states = append(states, state)
	}
	return states
}

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
	if _, err := platformservices.ResolveServiceBuildDirectory("/repository", req.RootDirectory); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_root_directory"})
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
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
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
			bindings, err := s.store.ListServiceEnvironments(r.Context(), service.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_service_environments_failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"service": service, "bindings": bindings, "environment_states": s.serviceEnvironmentStates(r, service, bindings)})
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
			_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: item.ApplicationID, ServiceID: item.ID, EventType: "service", Status: "updated", Actor: "operator", Message: "Service updated", Detail: item.Name})
			writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": item})
		case http.MethodDelete:
			result, err := platformservices.DeleteServiceAndRuntime(r.Context(), s.log, s.cfg, s.store, service.ID)
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": publicAPIError(err, "delete_service_failed")})
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
	if len(parts) == 4 && parts[1] == "environments" {
		if parts[3] == "metrics" {
			s.handleServiceMetricsV2(w, r, service.ID, parts[2])
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		environment, err := s.store.GetEnvironment(r.Context(), parts[2])
		if err != nil || environment.ApplicationID != service.ApplicationID {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "environment_not_found"})
			return
		}
		switch parts[3] {
		case "deployments":
			s.handleServiceDeployActionV2(w, r, service.ID, environment.ID)
		case "stop", "restart":
			s.handleServiceRuntimeActionV2(w, r, service.ID, environment.ID, parts[3])
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
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
			_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: service.ApplicationID, ServiceID: service.ID, EnvironmentID: environment.ID, EventType: "configuration", Status: "updated", Actor: "operator", Message: "Service environment updated", Detail: item.Branch})
			writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "binding": item})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
}

func (s *server) handleServiceRuntimeActionV2(w http.ResponseWriter, r *http.Request, serviceID, environmentID, action string) {
	var (
		result platformservices.ServiceRuntimeResult
		err    error
	)
	if action == "stop" {
		result, err = platformservices.StopServiceEnvironment(r.Context(), s.store, serviceID, environmentID)
	} else {
		result, err = platformservices.RestartServiceEnvironment(r.Context(), s.store, serviceID, environmentID)
	}
	if err != nil {
		code := publicAPIError(err, action+"_failed")
		status := http.StatusBadGateway
		if code == "runtime_no_active_deployment" || code == "runtime_active_container_lookup_failed" {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"status": "error", "error": code})
		return
	}
	service, _ := s.store.GetService(r.Context(), serviceID)
	_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: service.ApplicationID, ServiceID: serviceID, EnvironmentID: environmentID, DeploymentID: result.DeploymentID, EventType: "runtime", Status: result.Status, Actor: "operator", Message: "Service " + result.Status, Detail: action})
	writeJSON(w, http.StatusOK, map[string]any{
		"status": result.Status, "service_id": serviceID, "environment_id": environmentID,
		"deployment_id": result.DeploymentID, "container_id": result.ContainerID,
	})
}
