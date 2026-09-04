package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	githubapp "github.com/furious-fury/HostForge/internal/github/app"
	"github.com/furious-fury/HostForge/internal/repository"
	platformservices "github.com/furious-fury/HostForge/internal/services"
)

func (s *server) serviceEnvironmentStates(r *http.Request, service repository.Service, bindings []repository.ServiceEnvironment) []map[string]any {
	states := make([]map[string]any, 0, len(bindings))
	onboarding, onboardingErr := s.store.GetOnboardingState(r.Context())
	routeSyncNeeded := false
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
		domains, domainErr := s.store.ListServiceDomains(r.Context(), service.ApplicationID, binding.EnvironmentID, service.ID)
		if domainErr == nil && len(domains) == 0 && binding.ActiveDeploymentID != "" {
			switch {
			case onboardingErr != nil:
				state["public_url_status"] = "platform_state_unavailable"
			case strings.TrimSpace(onboarding.PlatformDomain) == "":
				state["public_url_status"] = "platform_domain_required"
			default:
				generated, created, ensureErr := s.store.EnsurePlatformServiceDomain(r.Context(), service.ApplicationID, binding.EnvironmentID, service.ID)
				if ensureErr != nil {
					state["public_url_status"] = "platform_domain_generation_failed"
				} else if generated.DomainName != "" {
					domains = []repository.ServiceDomain{generated}
					routeSyncNeeded = routeSyncNeeded || created
				}
			}
		}
		if domainErr == nil && len(domains) > 0 {
			state["public_url"] = "https://" + domains[0].DomainName
			state["domains"] = domains
			state["public_url_status"] = "ready"
		}
		states = append(states, state)
	}
	// Route sync is a fire-and-forget reconcile notify, not a synchronous call
	// (ADR-0002 §6.1): there is no error path left to downgrade
	// public_url_status with. It stays "ready" and converges to published
	// within one reconcile pass.
	if routeSyncNeeded && s.routeNotifier != nil {
		s.routeNotifier.Notify()
	}
	return states
}

type serviceRequest struct {
	Name                 string `json:"name"`
	RepoURL              string `json:"repo_url"`
	GitHubInstallationID int64  `json:"github_installation_id"`
	EnvironmentID        string `json:"environment_id"`
	Branch               string `json:"branch"`
	AutoDeploy           bool   `json:"auto_deploy"`
	RootDirectory        string `json:"root_directory"`
	Runtime              string `json:"runtime"`
	InstallCmd           string `json:"install_cmd"`
	BuildCmd             string `json:"build_cmd"`
	StartCmd             string `json:"start_cmd"`
	InternalPort         int    `json:"internal_port"`
	HealthCheckPath      string `json:"health_check_path"`
}

type githubRepositoryLister interface {
	ListInstallationRepositories(context.Context, int64) ([]githubapp.Repository, error)
	ListRepositoryBranches(context.Context, int64, string, string) ([]string, error)
}

func decodeServiceRequest(w http.ResponseWriter, r *http.Request, applicationID string, current *repository.Service) (repository.CreateServiceInput, bool, bool) {
	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return repository.CreateServiceInput{}, false, false
	}
	var req serviceRequest
	var present map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil || json.Unmarshal(body, &present) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return repository.CreateServiceInput{}, false, false
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
		return repository.CreateServiceInput{}, false, false
	}
	runtime, install, build, start, code := platformservices.ValidateDeployFields(req.Runtime, req.InstallCmd, req.BuildCmd, req.StartCmd)
	if code != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": code})
		return repository.CreateServiceInput{}, false, false
	}
	if _, err := platformservices.ResolveServiceBuildDirectory("/repository", req.RootDirectory); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_root_directory"})
		return repository.CreateServiceInput{}, false, false
	}
	if req.InternalPort < 1 || req.InternalPort > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_internal_port"})
		return repository.CreateServiceInput{}, false, false
	}
	currentRepoURL := ""
	if current != nil {
		currentRepoURL = current.RepoURL
		if canonical, err := platformservices.CanonicalRepoURL(currentRepoURL); err == nil {
			currentRepoURL = canonical
		}
	}
	sourceChanged := current == nil || current.GitHubInstallationID != req.GitHubInstallationID || currentRepoURL != repoURL
	return repository.CreateServiceInput{
		ApplicationID: applicationID, Name: req.Name, RepoURL: repoURL,
		GitHubInstallationID: req.GitHubInstallationID, RootDirectory: req.RootDirectory,
		DeployRuntime: runtime, InstallCmd: install, BuildCmd: build, StartCmd: start,
		InternalPort: req.InternalPort, HealthCheckPath: req.HealthCheckPath,
		InitialEnvironmentID: req.EnvironmentID, InitialBranch: req.Branch, InitialAutoDeploy: req.AutoDeploy,
	}, sourceChanged, true
}

func (s *server) validateServiceSource(w http.ResponseWriter, r *http.Request, in repository.CreateServiceInput) bool {
	if !s.validateGitHubInstallation(w, r, in.GitHubInstallationID) {
		return false
	}
	lister, ok := s.githubRepositoryAPI(w, r)
	if !ok {
		return false
	}
	repositories, err := lister.ListInstallationRepositories(r.Context(), in.GitHubInstallationID)
	if err != nil {
		s.requestLog(r).Error("validate service repository failed", "installation_id", in.GitHubInstallationID, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "repositories_list_failed"})
		return false
	}
	for _, candidate := range repositories {
		canonical, err := platformservices.CanonicalRepoURL(candidate.CloneURL)
		if err == nil && canonical == in.RepoURL {
			return true
		}
	}
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"status": "error", "error": "repository_not_accessible", "fields": map[string]string{"repo_url": "not_accessible_by_installation"}})
	return false
}

func (s *server) validateGitHubInstallation(w http.ResponseWriter, r *http.Request, installationID int64) bool {
	if installationID <= 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"status": "error", "error": "github_installation_required", "fields": map[string]string{"github_installation_id": "required"}})
		return false
	}
	installation, err := s.store.GetGitHubInstallation(r.Context(), installationID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"status": "error", "error": "github_installation_not_found", "fields": map[string]string{"github_installation_id": "not_found"}})
		return false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "github_installation_lookup_failed"})
		return false
	}
	if strings.TrimSpace(installation.SuspendedAt) != "" {
		writeJSON(w, http.StatusConflict, map[string]any{"status": "error", "error": "github_installation_suspended", "fields": map[string]string{"github_installation_id": "suspended"}})
		return false
	}
	return true
}

func (s *server) githubRepositoryAPI(w http.ResponseWriter, r *http.Request) (githubRepositoryLister, bool) {
	lister := s.githubRepoLister
	if lister == nil {
		client, err := s.loadAppClient(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "app_client_load_failed"})
			return nil, false
		}
		if client == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "app_not_configured"})
			return nil, false
		}
		lister = client
	}
	return lister, true
}

func (s *server) validateServiceBranch(w http.ResponseWriter, r *http.Request, service repository.Service, branch string) bool {
	if !s.validateGitHubInstallation(w, r, service.GitHubInstallationID) {
		return false
	}
	parsed, err := url.Parse(service.RepoURL)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"status": "error", "error": "invalid_github_repository", "fields": map[string]string{"repo_url": "github_repository_required"}})
		return false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"status": "error", "error": "invalid_github_repository", "fields": map[string]string{"repo_url": "github_repository_required"}})
		return false
	}
	parts[1] = strings.TrimSuffix(parts[1], ".git")
	lister, ok := s.githubRepositoryAPI(w, r)
	if !ok {
		return false
	}
	branches, err := lister.ListRepositoryBranches(r.Context(), service.GitHubInstallationID, parts[0], parts[1])
	if err != nil {
		s.requestLog(r).Error("validate service branch failed", "service_id", service.ID, "installation_id", service.GitHubInstallationID, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "repository_branches_list_failed"})
		return false
	}
	for _, candidate := range branches {
		if candidate == branch {
			return true
		}
	}
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"status": "error", "error": "branch_not_accessible", "fields": map[string]string{"branch": "not_found_in_repository"}})
	return false
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
			if service.ServiceType == "database" {
				databaseService, err := s.store.GetDatabaseService(r.Context(), service.ID)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "database_service_lookup_failed"})
					return
				}
				instances, err := s.store.ListDatabaseInstances(r.Context(), service.ID)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "database_instances_lookup_failed"})
					return
				}
				bindings := make(map[string][]repository.DatabaseBinding, len(instances))
				credentials := make(map[string]repository.DatabaseCredential, len(instances))
				for _, instance := range instances {
					items, err := s.store.ListDatabaseBindings(r.Context(), instance.ID)
					if err != nil {
						writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "database_bindings_lookup_failed"})
						return
					}
					bindings[instance.ID] = items
					credential, err := s.store.GetDatabaseCredentialSealed(r.Context(), instance.ID)
					if err != nil {
						writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "database_credentials_lookup_failed"})
						return
					}
					credentials[instance.ID] = credential
				}
				operations, err := s.store.ListDatabaseOperations(r.Context(), service.ID, 50)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "database_operations_lookup_failed"})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"service": service, "database": databaseService,
					"database_instances": instances, "database_bindings": bindings, "database_credentials": credentials, "database_operations": operations,
					"bindings": []repository.ServiceEnvironment{}, "environment_states": []any{},
				})
				return
			}
			bindings, err := s.store.ListServiceEnvironments(r.Context(), service.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_service_environments_failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"service": service, "bindings": bindings, "environment_states": s.serviceEnvironmentStates(r, service, bindings)})
		case http.MethodPatch:
			if service.ServiceType != "application" {
				writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_service_settings_endpoint_required"})
				return
			}
			in, sourceChanged, ok := decodeServiceRequest(w, r, service.ApplicationID, &service)
			if !ok {
				return
			}
			if sourceChanged && !s.validateServiceSource(w, r, in) {
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
			if service.ServiceType == "database" {
				var req struct {
					Confirmation string `json:"confirmation"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
					return
				}
				if strings.TrimSpace(req.Confirmation) != service.Name {
					writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "database_delete_confirmation_mismatch"})
					return
				}
				result, err := platformservices.DeleteDatabaseServiceAndRuntime(r.Context(), s.log, s.cfg, s.store, s.envSealer, s.dockerClient, service.ID, "operator", s.routeNotifier)
				if err != nil {
					writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": publicAPIError(err, "delete_database_service_failed")})
					return
				}
				_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{
					ApplicationID: service.ApplicationID, ServiceID: service.ID,
					EventType: "database", Status: "deleted", Actor: "operator",
					Message: "Database deleted; volumes retained", Detail: result.PurgeAfter.Format(time.RFC3339),
				})
				writeJSON(w, http.StatusOK, map[string]any{
					"status": "deleted", "retained": true, "purge_after": result.PurgeAfter,
				})
				return
			}
			_, err := platformservices.DeleteServiceAndRuntime(r.Context(), s.log, s.cfg, s.store, s.dockerClient, service.ID, s.routeNotifier)
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": publicAPIError(err, "delete_service_failed")})
				return
			}
			_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: service.ApplicationID, EventType: "service", Status: "deleted", Actor: "operator", Message: "Service deleted", Detail: service.Name})
			writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	if len(parts) == 4 && parts[1] == "environments" {
		if service.ServiceType != "application" {
			writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "service_type_not_deployable"})
			return
		}
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
		if service.ServiceType != "application" {
			writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "service_type_not_deployable"})
			return
		}
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
			current, err := s.store.GetServiceEnvironment(r.Context(), service.ID, environment.ID)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			if req.Branch != "" && req.Branch != current.Branch && !s.validateServiceBranch(w, r, service, req.Branch) {
				return
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
		result, err = platformservices.StopServiceEnvironment(r.Context(), s.store, s.dockerClient, serviceID, environmentID)
	} else {
		result, err = platformservices.RestartServiceEnvironment(r.Context(), s.store, s.dockerClient, serviceID, environmentID)
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
