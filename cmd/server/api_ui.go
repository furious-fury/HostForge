package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/git"
	"github.com/hostforge/hostforge/internal/models"
	"github.com/hostforge/hostforge/internal/repository"
	"github.com/hostforge/hostforge/internal/services"
)

type createProjectRequest struct {
	RepoURL     string `json:"repo_url"`
	Branch      string `json:"branch"`
	ProjectName string `json:"project_name"`
}

type deploymentActionResponse struct {
	Status       string `json:"status"`
	Mode         string `json:"mode,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	DeploymentID string `json:"deployment_id,omitempty"`
	ContainerID  string `json:"container_id,omitempty"`
	URL          string `json:"url,omitempty"`
	Error        string `json:"error,omitempty"`
}

type apiProject struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	RepoURL          string         `json:"repo_url"`
	Branch           string         `json:"branch"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at"`
	LatestDeployment *apiDeployment `json:"latest_deployment,omitempty"`
	Domains          []apiDomain    `json:"domains,omitempty"`
	CurrentContainer *apiContainer  `json:"current_container,omitempty"`
}

type apiDeployment struct {
	ID           string        `json:"id"`
	ProjectID    string        `json:"project_id"`
	Status       string        `json:"status"`
	CommitHash   string        `json:"commit_hash"`
	LogsPath     string        `json:"logs_path"`
	ImageRef     string        `json:"image_ref"`
	Worktree     string        `json:"worktree"`
	ErrorMessage string        `json:"error_message"`
	CreatedAt    string        `json:"created_at"`
	UpdatedAt    string        `json:"updated_at"`
	Container    *apiContainer `json:"container,omitempty"`
}

type apiContainer struct {
	ID                string `json:"id"`
	DeploymentID      string `json:"deployment_id"`
	DockerContainerID string `json:"docker_container_id"`
	InternalPort      int    `json:"internal_port"`
	HostPort          int    `json:"host_port"`
	Status            string `json:"status"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type apiDomain struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	DomainName string `json:"domain_name"`
	SSLStatus  string `json:"ssl_status"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type repositoryBranchesResponse struct {
	Status        string   `json:"status"`
	RepoURL       string   `json:"repo_url"`
	Branches      []string `json:"branches"`
	DefaultBranch string   `json:"default_branch"`
}

func (s *server) handleProjectsCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleProjectsList(w, r)
	case http.MethodPost:
		s.handleProjectCreate(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
	}
}

func (s *server) handleRepositoryBranches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	repoRaw := strings.TrimSpace(r.URL.Query().Get("repo_url"))
	if repoRaw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "missing_repo_url"})
		return
	}
	repoURL, err := services.CanonicalRepoURL(repoRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_repository_clone_url"})
		return
	}
	branches, inferredDefault, err := git.ListRemoteBranches(r.Context(), repoURL)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "list_remote_branches_failed"})
		return
	}
	defaultBranch := git.ResolveBranch(r.Context(), repoURL, "")
	writeJSON(w, http.StatusOK, repositoryBranchesResponse{
		Status:        "ok",
		RepoURL:       repoURL,
		Branches:      branches,
		DefaultBranch: firstNonEmpty(inferredDefault, defaultBranch),
	})
}

func (s *server) handleProjectsList(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListProjects(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_projects_failed"})
		return
	}
	out := make([]apiProject, 0, len(items))
	for _, p := range items {
		apiItem := projectToAPI(p)
		if err := s.attachProjectSummary(r.Context(), &apiItem); err != nil {
			s.log.Warn("failed to build project summary", "project_id", p.ID, "error", err)
		}
		out = append(out, apiItem)
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

func (s *server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"status": "error", "error": "content_type_must_be_application_json"})
		return
	}
	defer r.Body.Close()
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	repoURL, err := services.CanonicalRepoURL(req.RepoURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_repository_clone_url"})
		return
	}
	branch := strings.TrimSpace(req.Branch)
	branch = git.ResolveBranch(r.Context(), repoURL, branch)
	name := strings.TrimSpace(req.ProjectName)
	if name == "" {
		name = inferProjectName(repoURL)
	}
	if existing, err := s.store.GetProjectByRepoAndBranch(r.Context(), repoURL, branch); err == nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"status":  "error",
			"error":   "project_already_exists_for_repo_branch",
			"project": projectToAPI(existing),
		})
		return
	} else if !errorsIsNoRows(err) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}

	project, err := s.store.CreateProject(r.Context(), repository.CreateProjectInput{
		Name:    name,
		RepoURL: repoURL,
		Branch:  branch,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "create_project_failed"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"status":  "created",
		"project": projectToAPI(project),
	})
}

func (s *server) handleProjectRoutes(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		http.NotFound(w, r)
		return
	}
	projectID := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.handleProjectGet(w, r, projectID)
		case http.MethodDelete:
			s.handleProjectDelete(w, r, projectID)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	switch parts[1] {
	case "domains":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleProjectDomainsGet(w, r, projectID)
	case "deployments":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleProjectDeploymentsGet(w, r, projectID)
	case "deploy":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleProjectDeployAction(w, r, projectID)
	case "restart":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleProjectRestartAction(w, r, projectID)
	case "stop":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleProjectStopAction(w, r, projectID)
	case "rollback":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleProjectRollbackAction(w, r, projectID)
	default:
		http.NotFound(w, r)
	}
}

func (s *server) handleProjectDelete(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	if err := services.DeleteProject(r.Context(), s.log.With("project_id", projectID), s.cfg, s.store, projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		s.log.Error("delete project failed", "project_id", projectID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "delete_project_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "project_id": projectID})
}

func (s *server) handleProjectGet(w http.ResponseWriter, r *http.Request, projectID string) {
	project, err := s.store.GetProjectByID(r.Context(), projectID)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	resp := projectToAPI(project)
	if err := s.attachProjectSummary(r.Context(), &resp); err != nil {
		s.log.Warn("failed to load project summary", "project_id", projectID, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": resp})
}

func (s *server) handleProjectDomainsGet(w http.ResponseWriter, r *http.Request, projectID string) {
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	domains, err := s.store.ListDomainsByProject(r.Context(), projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_domains_failed"})
		return
	}
	out := make([]apiDomain, 0, len(domains))
	for _, d := range domains {
		out = append(out, domainToAPI(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": out})
}

func (s *server) handleProjectDeploymentsGet(w http.ResponseWriter, r *http.Request, projectID string) {
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	limit := parseQueryInt(r, "limit", 100)
	items, err := s.store.ListDeploymentsByProjectID(r.Context(), projectID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_deployments_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": s.enrichDeploymentsWithContainers(r.Context(), items)})
}

func (s *server) handleDeploymentsCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	limit := parseQueryInt(r, "limit", 100)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	all, err := s.store.ListDeployments(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_deployments_failed"})
		return
	}
	if len(all) > limit {
		all = all[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": s.enrichDeploymentsWithContainers(r.Context(), all)})
}

func (s *server) handleProjectDeployAction(w http.ResponseWriter, r *http.Request, projectID string) {
	project, err := s.store.GetProjectByID(r.Context(), projectID)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	requestID := newRequestID()
	deployLog := s.log.With("request_id", requestID, "project_id", project.ID)
	job, err := services.PrepareDeploy(r.Context(), s.cfg, s.store, services.DeployPrepareInput{
		Project: project,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "failed_to_accept_deployment"})
		return
	}
	deployLog = deployLog.With("deployment_id", job.Deployment.ID, "repo_url", project.RepoURL, "branch", project.Branch)
	asyncRequested := s.cfg.WebhookAsync || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("async")), "true") || r.URL.Query().Get("async") == "1"
	if asyncRequested {
		go func(job services.DeployJob) {
			if _, execErr := services.ExecuteDeploy(context.Background(), deployLog, s.cfg, s.store, job); execErr != nil {
				deployLog.Error("async deployment failed", "error", execErr)
			}
		}(job)
		writeJSON(w, http.StatusAccepted, deploymentActionResponse{
			Status:       "accepted",
			Mode:         "async",
			ProjectID:    project.ID,
			DeploymentID: job.Deployment.ID,
		})
		return
	}
	result, err := services.ExecuteDeploy(r.Context(), deployLog, s.cfg, s.store, job)
	if err != nil {
		writeJSON(w, http.StatusOK, deploymentActionResponse{
			Status:       "failed",
			Mode:         "sync",
			ProjectID:    project.ID,
			DeploymentID: job.Deployment.ID,
			Error:        err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, deploymentActionResponse{
		Status:       "success",
		Mode:         "sync",
		ProjectID:    project.ID,
		DeploymentID: result.DeploymentID,
		ContainerID:  result.ContainerID,
		URL:          result.URL,
	})
}

func (s *server) handleProjectRestartAction(w http.ResponseWriter, r *http.Request, projectID string) {
	project, err := s.store.GetProjectByID(r.Context(), projectID)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	result, err := services.RestartProject(r.Context(), s.log.With("project_id", projectID), s.cfg, s.store, project)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "active_container_not_found"})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": err.Error()})
		return
	}
	containerRec, err := s.store.GetContainerByDeploymentID(r.Context(), result.DeploymentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "post_restart_lookup_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "restarted",
		"project_id":    projectID,
		"container":     containerToAPI(containerRec),
		"url":           result.URL,
		"host_port":     result.HostPort,
		"recreated":     result.Recreated,
		"deployment_id": result.DeploymentID,
	})
}

func (s *server) handleProjectStopAction(w http.ResponseWriter, r *http.Request, projectID string) {
	_, activeContainer, err := s.resolveActiveProjectContainer(r.Context(), projectID)
	if err != nil {
		s.writeProjectContainerLookupError(w, err)
		return
	}
	cli, err := docker.NewClient(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "docker_unavailable"})
		return
	}
	defer cli.Close()
	if err := docker.StopContainer(r.Context(), cli, activeContainer.DockerContainerID); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "stop_container_failed"})
		return
	}
	_ = s.store.UpdateContainerStatus(r.Context(), activeContainer.ID, "STOPPED")
	activeContainer.Status = "STOPPED"
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "stopped",
		"project_id": projectID,
		"container":  containerToAPI(activeContainer),
	})
}

func (s *server) handleProjectRollbackAction(w http.ResponseWriter, r *http.Request, projectID string) {
	project, err := s.store.GetProjectByID(r.Context(), projectID)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	result, err := services.RollbackProject(r.Context(), s.log.With("project_id", projectID), s.cfg, s.store, project)
	if err != nil {
		status := http.StatusBadRequest
		if !errors.Is(err, sql.ErrNoRows) {
			status = http.StatusInternalServerError
		}
		writeJSON(w, status, map[string]string{"status": "error", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":             "rolled_back",
		"project_id":         projectID,
		"from_deployment_id": result.FromDeploymentID,
		"to_deployment_id":   result.ToDeploymentID,
		"container_id":       result.ContainerID,
		"url":                result.URL,
		"host_port":          result.HostPort,
	})
}

func (s *server) resolveActiveProjectContainer(ctx context.Context, projectID string) (models.Deployment, models.Container, error) {
	project, err := s.store.GetProjectByID(ctx, projectID)
	if err != nil {
		return models.Deployment{}, models.Container{}, err
	}
	deployment, err := s.store.GetLatestSuccessfulDeploymentByProjectID(ctx, project.ID)
	if err != nil {
		return models.Deployment{}, models.Container{}, err
	}
	containerRec, err := s.store.GetContainerByDeploymentID(ctx, deployment.ID)
	if err != nil {
		return models.Deployment{}, models.Container{}, err
	}
	return deployment, containerRec, nil
}

func (s *server) writeProjectContainerLookupError(w http.ResponseWriter, err error) {
	if errorsIsNoRows(err) {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "active_container_not_found"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "lookup_active_container_failed"})
}

func (s *server) attachProjectSummary(ctx context.Context, out *apiProject) error {
	domains, err := s.store.ListDomainsByProject(ctx, out.ID)
	if err != nil {
		return fmt.Errorf("project domains: %w", err)
	}
	out.Domains = make([]apiDomain, 0, len(domains))
	for _, d := range domains {
		out.Domains = append(out.Domains, domainToAPI(d))
	}
	deployments, err := s.store.ListDeploymentsByProjectID(ctx, out.ID, 1)
	if err != nil {
		return fmt.Errorf("latest deployment: %w", err)
	}
	if len(deployments) == 0 {
		return nil
	}
	latest := deploymentToAPI(deployments[0])
	if containerRec, err := s.store.GetContainerByDeploymentID(ctx, deployments[0].ID); err == nil {
		c := containerToAPI(containerRec)
		latest.Container = &c
		out.CurrentContainer = &c
	}
	out.LatestDeployment = &latest
	return nil
}

func (s *server) enrichDeploymentsWithContainers(ctx context.Context, items []models.Deployment) []apiDeployment {
	out := make([]apiDeployment, 0, len(items))
	for _, d := range items {
		item := deploymentToAPI(d)
		if containerRec, err := s.store.GetContainerByDeploymentID(ctx, d.ID); err == nil {
			c := containerToAPI(containerRec)
			item.Container = &c
		}
		out = append(out, item)
	}
	return out
}

func projectToAPI(p models.Project) apiProject {
	return apiProject{
		ID:        p.ID,
		Name:      p.Name,
		RepoURL:   p.RepoURL,
		Branch:    p.Branch,
		CreatedAt: formatTime(p.CreatedAt),
		UpdatedAt: formatTime(p.UpdatedAt),
	}
}

func deploymentToAPI(d models.Deployment) apiDeployment {
	return apiDeployment{
		ID:           d.ID,
		ProjectID:    d.ProjectID,
		Status:       d.Status,
		CommitHash:   d.CommitHash,
		LogsPath:     d.LogsPath,
		ImageRef:     d.ImageRef,
		Worktree:     d.Worktree,
		ErrorMessage: d.ErrorMessage,
		CreatedAt:    formatTime(d.CreatedAt),
		UpdatedAt:    formatTime(d.UpdatedAt),
	}
}

func containerToAPI(c models.Container) apiContainer {
	return apiContainer{
		ID:                c.ID,
		DeploymentID:      c.DeploymentID,
		DockerContainerID: c.DockerContainerID,
		InternalPort:      c.InternalPort,
		HostPort:          c.HostPort,
		Status:            c.Status,
		CreatedAt:         formatTime(c.CreatedAt),
		UpdatedAt:         formatTime(c.UpdatedAt),
	}
}

func domainToAPI(d models.Domain) apiDomain {
	return apiDomain{
		ID:         d.ID,
		ProjectID:  d.ProjectID,
		DomainName: d.DomainName,
		SSLStatus:  d.SSLStatus,
		CreatedAt:  formatTime(d.CreatedAt),
		UpdatedAt:  formatTime(d.UpdatedAt),
	}
}

func inferProjectName(repoURL string) string {
	trimmed := strings.TrimSpace(repoURL)
	trimmed = strings.TrimSuffix(trimmed, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return "project"
	}
	name := strings.TrimSuffix(parts[len(parts)-1], ".git")
	name = strings.TrimSpace(name)
	if name == "" {
		return "project"
	}
	return name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
