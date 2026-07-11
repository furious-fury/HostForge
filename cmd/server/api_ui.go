package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/dnsops"
	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/git"
	"github.com/hostforge/hostforge/internal/models"
	"github.com/hostforge/hostforge/internal/obs"
	"github.com/hostforge/hostforge/internal/redact"
	"github.com/hostforge/hostforge/internal/repository"
	"github.com/hostforge/hostforge/internal/services"
)

type apiDeployConfig struct {
	Runtime    string `json:"runtime"`
	InstallCmd string `json:"install_cmd"`
	BuildCmd   string `json:"build_cmd"`
	StartCmd   string `json:"start_cmd"`
}

type createProjectRequest struct {
	RepoURL              string           `json:"repo_url"`
	Branch               string           `json:"branch"`
	ProjectName          string           `json:"project_name"`
	GitSource            string           `json:"git_source,omitempty"`
	GitHubInstallationID int64            `json:"github_installation_id,omitempty"`
	Deploy               *apiDeployConfig `json:"deploy,omitempty"`
	Env                  []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"env,omitempty"`
}

type patchProjectRequest struct {
	Deploy *apiDeployConfig `json:"deploy,omitempty"`
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
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	RepoURL              string          `json:"repo_url"`
	Branch               string          `json:"branch"`
	GitSource            string          `json:"git_source,omitempty"`
	GitHubInstallationID int64           `json:"github_installation_id,omitempty"`
	Deploy               apiDeployConfig `json:"deploy"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
	// StackKind and StackLabel mirror the latest deployment row when present (convenience for list UIs).
	StackKind        string           `json:"stack_kind,omitempty"`
	StackLabel       string           `json:"stack_label,omitempty"`
	LatestDeployment *apiDeployment   `json:"latest_deployment,omitempty"`
	Domains          []apiDomain      `json:"domains,omitempty"`
	DNSGuidance      *dnsops.Guidance `json:"dns_guidance,omitempty"`
	CurrentContainer *apiContainer    `json:"current_container,omitempty"`
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
	StackKind    string        `json:"stack_kind,omitempty"`
	StackLabel   string        `json:"stack_label,omitempty"`
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

type apiProjectEnvVar struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	ValueLast4 string `json:"value_last4"`
	UpdatedAt  string `json:"updated_at"`
}

type apiProjectGitAuth struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider,omitempty"`
	TokenLast4 string `json:"token_last4,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type apiDomain struct {
	ID                 string   `json:"id"`
	ProjectID          string   `json:"project_id"`
	DomainName         string   `json:"domain_name"`
	SSLStatus          string   `json:"ssl_status"`                  // Caddy route state: ACTIVE = snippet applied, not "HTTPS works publicly"
	LastCertMessage    string   `json:"last_cert_message,omitempty"` // optional cert poll summary (see README)
	CertCheckedAt      string   `json:"cert_checked_at,omitempty"`   // RFC3339 when last cert poll ran for this row
	RegistrarDNSStatus string   `json:"registrar_dns_status"`        // ok | pending | unknown | lookup_error — public DNS vs expected server IPv4
	ResolvedIPv4       []string `json:"resolved_ipv4,omitempty"`
	CreatedAt          string   `json:"created_at"`
	UpdatedAt          string   `json:"updated_at"`
}

type caddySyncOutcome struct {
	Attempted bool   `json:"attempted"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"` // stable machine-oriented code when attempted && !ok
}

type domainNameRequest struct {
	DomainName string `json:"domain_name"`
}

type repositoryBranchesResponse struct {
	Status        string   `json:"status"`
	RepoURL       string   `json:"repo_url"`
	Branches      []string `json:"branches"`
	DefaultBranch string   `json:"default_branch"`
}

func mapDomainValidationError(err error) string {
	switch {
	case errors.Is(err, dnsops.ErrDomainNameEmpty):
		return "domain_name_empty"
	case errors.Is(err, dnsops.ErrDomainNameTooLong):
		return "domain_name_too_long"
	case errors.Is(err, dnsops.ErrDomainNameInvalid):
		return "domain_name_invalid"
	default:
		return "invalid_domain_name"
	}
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
	gitAuth := git.AuthOptions{}
	if projectID := strings.TrimSpace(r.URL.Query().Get("project_id")); projectID != "" {
		project, err := s.store.GetProjectByID(r.Context(), projectID)
		if err != nil {
			if errorsIsNoRows(err) {
				writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
			return
		}
		opts, err := s.resolveGitAuthForProject(r.Context(), project)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "git_auth_prepare_failed"})
			return
		}
		gitAuth = opts
	} else if raw := strings.TrimSpace(r.URL.Query().Get("installation_id")); raw != "" {
		installationID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || installationID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_installation_id"})
			return
		}
		opts, err := s.resolveGitAuthForInstallation(r.Context(), installationID)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "installation_token_mint_failed"})
			return
		}
		gitAuth = opts
	}
	branches, inferredDefault, err := git.ListRemoteBranches(r.Context(), repoURL, gitAuth)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "list_remote_branches_failed"})
		return
	}
	defaultBranch := git.ResolveBranch(r.Context(), repoURL, "", gitAuth)
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
	// Default: fast list (DB + latest deploy only). Use ?dns=1 for live registrar checks + dns_guidance per project.
	fullDNS := strings.EqualFold(r.URL.Query().Get("dns"), "true") ||
		r.URL.Query().Get("dns") == "1" ||
		strings.EqualFold(r.URL.Query().Get("full_dns"), "1")
	out := make([]apiProject, 0, len(items))
	for _, p := range items {
		apiItem := projectToAPI(p)
		if err := s.attachProjectSummary(r.Context(), &apiItem, fullDNS); err != nil {
			s.requestLog(r).Warn("failed to build project summary", "project_id", p.ID, "error", err)
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
	gitSource := strings.TrimSpace(req.GitSource)
	if gitSource == "" {
		gitSource = models.GitSourceURL
	}
	switch gitSource {
	case models.GitSourceURL, models.GitSourceGitHubApp, models.GitSourceSSH:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_git_source"})
		return
	}
	installationID := req.GitHubInstallationID
	if gitSource == models.GitSourceGitHubApp {
		if installationID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "github_installation_id_required"})
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

	branchAuth := git.AuthOptions{}
	if installationID > 0 {
		if opts, err := s.resolveGitAuthForInstallation(r.Context(), installationID); err == nil {
			branchAuth = opts
		}
	}
	branch := strings.TrimSpace(req.Branch)
	branch = git.ResolveBranch(r.Context(), repoURL, branch, branchAuth)
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

	deployIn := repository.CreateProjectInput{
		Name:                 name,
		RepoURL:              repoURL,
		Branch:               branch,
		DeployRuntime:        models.DeployRuntimeAuto,
		DeployInstallCmd:     "",
		DeployBuildCmd:       "",
		DeployStartCmd:       "",
		GitSource:            gitSource,
		GitHubInstallationID: installationID,
	}
	if req.Deploy != nil {
		rt, i, b, st, errCode := services.ValidateDeployFields(req.Deploy.Runtime, req.Deploy.InstallCmd, req.Deploy.BuildCmd, req.Deploy.StartCmd)
		if errCode != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": errCode})
			return
		}
		deployIn.DeployRuntime = rt
		deployIn.DeployInstallCmd = i
		deployIn.DeployBuildCmd = b
		deployIn.DeployStartCmd = st
	}

	filteredEnv := filterProjectEnvPairs(req.Env)
	var project models.Project
	var createErr error
	if len(filteredEnv) == 0 {
		project, createErr = s.store.CreateProject(r.Context(), deployIn)
		if createErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "create_project_failed"})
			return
		}
	} else {
		if !s.requireEnvSealer(w) {
			return
		}
		sealed, errCode := sealProjectEnvBatch(s.envSealer, filteredEnv)
		if errCode != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": errCode})
			return
		}
		project, createErr = s.store.CreateProjectWithSealedEnv(r.Context(), deployIn, sealed)
		if createErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "create_project_failed"})
			return
		}
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
		case http.MethodPatch:
			s.handleProjectPatch(w, r, projectID)
		case http.MethodDelete:
			s.handleProjectDelete(w, r, projectID)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	switch parts[1] {
	case "git-auth":
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.handleProjectGitAuthGet(w, r, projectID)
		case http.MethodPut:
			s.handleProjectGitAuthPut(w, r, projectID)
		case http.MethodDelete:
			s.handleProjectGitAuthDelete(w, r, projectID)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	case "ssh-key":
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.handleProjectSSHKeyGet(w, r, projectID)
		case http.MethodPost:
			s.handleProjectSSHKeyGenerate(w, r, projectID)
		case http.MethodDelete:
			s.handleProjectSSHKeyDelete(w, r, projectID)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	case "git-source":
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPut:
			s.handleProjectGitSourcePut(w, r, projectID)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	case "env":
		if len(parts) == 2 {
			switch r.Method {
			case http.MethodGet:
				s.handleProjectEnvList(w, r, projectID)
			case http.MethodPost:
				s.handleProjectEnvPost(w, r, projectID)
			default:
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			}
			return
		}
		if len(parts) == 3 {
			envID := strings.TrimSpace(parts[2])
			if envID == "" {
				http.NotFound(w, r)
				return
			}
			switch r.Method {
			case http.MethodPut:
				s.handleProjectEnvPut(w, r, projectID, envID)
			case http.MethodDelete:
				s.handleProjectEnvDelete(w, r, projectID, envID)
			default:
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			}
			return
		}
		http.NotFound(w, r)
		return
	case "domains":
		if len(parts) == 2 {
			switch r.Method {
			case http.MethodGet:
				s.handleProjectDomainsCollection(w, r, projectID)
			case http.MethodPost:
				s.handleProjectDomainsPost(w, r, projectID)
			default:
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			}
			return
		}
		if len(parts) == 3 {
			domainID := strings.TrimSpace(parts[2])
			if domainID == "" {
				http.NotFound(w, r)
				return
			}
			switch r.Method {
			case http.MethodPatch:
				s.handleProjectDomainPatch(w, r, projectID, domainID)
			case http.MethodDelete:
				s.handleProjectDomainDelete(w, r, projectID, domainID)
			default:
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			}
			return
		}
		http.NotFound(w, r)
		return
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
	if err := services.DeleteProject(r.Context(), s.requestLog(r).With("project_id", projectID), s.cfg, s.store, projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		s.requestLog(r).Error("delete project failed", "project_id", projectID, "error", err)
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
	if err := s.attachProjectSummary(r.Context(), &resp, true); err != nil {
		s.requestLog(r).Warn("failed to load project summary", "project_id", projectID, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": resp})
}

func (s *server) handleProjectPatch(w http.ResponseWriter, r *http.Request, projectID string) {
	if !strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"status": "error", "error": "content_type_must_be_application_json"})
		return
	}
	defer r.Body.Close()
	var req patchProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	if req.Deploy == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "missing_deploy"})
		return
	}
	rt, i, b, st, errCode := services.ValidateDeployFields(req.Deploy.Runtime, req.Deploy.InstallCmd, req.Deploy.BuildCmd, req.Deploy.StartCmd)
	if errCode != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": errCode})
		return
	}
	project, err := s.store.UpdateProjectDeployConfig(r.Context(), projectID, rt, i, b, st)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "update_project_failed"})
		return
	}
	resp := projectToAPI(project)
	if err := s.attachProjectSummary(r.Context(), &resp, true); err != nil {
		s.requestLog(r).Warn("failed to load project summary", "project_id", projectID, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "project": resp})
}

func (s *server) handleProjectDomainsCollection(w http.ResponseWriter, r *http.Request, projectID string) {
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
	expectedIPv4, v4src, v4warn := dnsops.ResolveExpectedIPv4(r.Context(), s.cfg)
	out := make([]apiDomain, 0, len(domains))
	names := make([]string, 0, len(domains))
	for _, d := range domains {
		out = append(out, s.domainToAPIExpected(r.Context(), d, expectedIPv4))
		names = append(names, d.DomainName)
	}
	g := dnsops.BuildGuidanceWithIPv4(r.Context(), s.cfg, names, expectedIPv4, v4src, v4warn)
	writeJSON(w, http.StatusOK, map[string]any{"domains": out, "dns_guidance": g})
}

func (s *server) handleProjectDomainsPost(w http.ResponseWriter, r *http.Request, projectID string) {
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	var body domainNameRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json"})
		return
	}
	if err := dnsops.ValidateDomainName(body.DomainName); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": mapDomainValidationError(err)})
		return
	}
	d, err := s.store.CreateDomain(r.Context(), projectID, body.DomainName)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateDomain) {
			writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "duplicate_domain"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "create_domain_failed"})
		return
	}
	syncOut := s.caddySyncAfterDomainChange(r.Context(), s.requestLog(r))
	expectedIPv4, v4src, v4warn := dnsops.ResolveExpectedIPv4(r.Context(), s.cfg)
	g := dnsops.BuildGuidanceWithIPv4(r.Context(), s.cfg, []string{d.DomainName}, expectedIPv4, v4src, v4warn)
	writeJSON(w, http.StatusCreated, map[string]any{
		"status":       "created",
		"domain":       s.domainToAPIExpected(r.Context(), d, expectedIPv4),
		"dns_guidance": g,
		"caddy_sync":   syncOut,
	})
}

func (s *server) handleProjectDomainPatch(w http.ResponseWriter, r *http.Request, projectID, domainID string) {
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	var body domainNameRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json"})
		return
	}
	if err := dnsops.ValidateDomainName(body.DomainName); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": mapDomainValidationError(err)})
		return
	}
	d, err := s.store.UpdateDomainName(r.Context(), projectID, domainID, body.DomainName)
	if err != nil {
		if errors.Is(err, repository.ErrDomainNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "domain_not_found"})
			return
		}
		if errors.Is(err, repository.ErrDuplicateDomain) {
			writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "duplicate_domain"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "update_domain_failed"})
		return
	}
	syncOut := s.caddySyncAfterDomainChange(r.Context(), s.requestLog(r))
	expectedIPv4, v4src, v4warn := dnsops.ResolveExpectedIPv4(r.Context(), s.cfg)
	g := dnsops.BuildGuidanceWithIPv4(r.Context(), s.cfg, []string{d.DomainName}, expectedIPv4, v4src, v4warn)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"domain":       s.domainToAPIExpected(r.Context(), d, expectedIPv4),
		"dns_guidance": g,
		"caddy_sync":   syncOut,
	})
}

func (s *server) handleProjectDomainDelete(w http.ResponseWriter, r *http.Request, projectID, domainID string) {
	if _, err := s.store.GetProjectByID(r.Context(), projectID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "project_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "project_lookup_failed"})
		return
	}
	existing, err := s.store.GetDomainByID(r.Context(), domainID)
	if err != nil {
		if errors.Is(err, repository.ErrDomainNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "domain_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "domain_lookup_failed"})
		return
	}
	if existing.ProjectID != projectID {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "domain_not_found"})
		return
	}
	if err := s.store.DeleteDomain(r.Context(), domainID); err != nil {
		if errors.Is(err, repository.ErrDomainNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "domain_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "delete_domain_failed"})
		return
	}
	syncOut := s.caddySyncAfterDomainChange(r.Context(), s.requestLog(r))
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "deleted",
		"domain_id":  domainID,
		"caddy_sync": syncOut,
	})
}

func (s *server) caddySyncAfterDomainChange(ctx context.Context, lg *slog.Logger) caddySyncOutcome {
	out := caddySyncOutcome{}
	if !s.cfg.DomainSyncAfterMutate {
		return out
	}
	if strings.TrimSpace(s.cfg.CaddyRootConfig) == "" {
		return out
	}
	out.Attempted = true
	if lg == nil {
		lg = s.log
	}
	t0 := time.Now()
	if err := services.SyncCaddyRoutes(ctx, lg, s.cfg, s.store); err != nil {
		out.OK = false
		out.Error = publicAPIError(err, "caddy_sync_failed")
		lg.Warn("caddy sync after domain change failed", "error", err, "public_code", out.Error, "duration_ms", time.Since(t0).Milliseconds())
		return out
	}
	out.OK = true
	lg.Info("caddy sync after domain change complete", "duration_ms", time.Since(t0).Milliseconds())
	return out
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
	writeJSON(w, http.StatusOK, map[string]any{"deployments": s.enrichDeploymentsWithContainers(r, items)})
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
	all, err := s.store.ListDeploymentsRecent(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_deployments_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": s.enrichDeploymentsWithContainers(r, all)})
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
	deployLog := s.requestLog(r).With("project_id", project.ID)
	job, err := services.PrepareDeploy(r.Context(), s.cfg, s.store, services.DeployPrepareInput{
		Project: project,
	})
	if err != nil {
		deployLog.Error("prepare deploy failed", "error", err, "public_code", publicAPIError(err, "failed_to_accept_deployment"))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": publicAPIError(err, "failed_to_accept_deployment")})
		return
	}
	deployLog = deployLog.With("deployment_id", job.Deployment.ID, "repo_url", redact.RepoURLForLog(project.RepoURL), "branch", project.Branch)
	asyncRequested := s.cfg.WebhookAsync || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("async")), "true") || r.URL.Query().Get("async") == "1"
	if asyncRequested {
		resolver := s.newGitAuthResolver(r.Context())
		go func(job services.DeployJob) {
			bg := obs.WithStore(context.Background(), s.store)
			if _, execErr := services.ExecuteDeploy(bg, deployLog, s.cfg, s.store, job, s.envSealer, resolver); execErr != nil {
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
	tExec := time.Now()
	result, err := services.ExecuteDeploy(r.Context(), deployLog, s.cfg, s.store, job, s.envSealer, s.newGitAuthResolver(r.Context()))
	if err != nil {
		deployLog.Error("execute deploy failed", "error", err, "public_code", publicAPIError(err, "deploy_failed"), "duration_ms", time.Since(tExec).Milliseconds())
		writeJSON(w, http.StatusOK, deploymentActionResponse{
			Status:       "failed",
			Mode:         "sync",
			ProjectID:    project.ID,
			DeploymentID: job.Deployment.ID,
			Error:        publicAPIError(err, "deploy_failed"),
		})
		return
	}
	deployLog.Info("execute deploy finished", "deployment_id", result.DeploymentID, "duration_ms", time.Since(tExec).Milliseconds())
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
	result, err := services.RestartProject(r.Context(), s.requestLog(r).With("project_id", projectID), s.cfg, s.store, project, s.envSealer)
	if err != nil {
		code := publicAPIError(err, "restart_failed")
		status := http.StatusBadGateway
		if errorsIsNoRows(err) {
			status = http.StatusNotFound
		}
		s.requestLog(r).Warn("restart failed", "project_id", projectID, "error", err, "public_code", code)
		writeJSON(w, status, map[string]string{"status": "error", "error": code})
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
	result, err := services.RollbackProject(r.Context(), s.requestLog(r).With("project_id", projectID), s.cfg, s.store, project, s.envSealer)
	if err != nil {
		code := publicAPIError(err, "rollback_failed")
		status := http.StatusBadRequest
		if !errors.Is(err, sql.ErrNoRows) {
			status = http.StatusInternalServerError
		}
		s.requestLog(r).Warn("rollback failed", "project_id", projectID, "error", err, "public_code", code)
		writeJSON(w, status, map[string]string{"status": "error", "error": code})
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

func (s *server) attachProjectSummary(ctx context.Context, out *apiProject, fullDNS bool) error {
	domains, err := s.store.ListDomainsByProject(ctx, out.ID)
	if err != nil {
		return fmt.Errorf("project domains: %w", err)
	}
	out.Domains = make([]apiDomain, 0, len(domains))
	if fullDNS {
		expectedIPv4, v4src, v4warn := dnsops.ResolveExpectedIPv4(ctx, s.cfg)
		names := make([]string, 0, len(domains))
		for _, d := range domains {
			out.Domains = append(out.Domains, s.domainToAPIExpected(ctx, d, expectedIPv4))
			names = append(names, d.DomainName)
		}
		if len(names) > 0 {
			g := dnsops.BuildGuidanceWithIPv4(ctx, s.cfg, names, expectedIPv4, v4src, v4warn)
			out.DNSGuidance = &g
		}
	} else {
		for _, d := range domains {
			out.Domains = append(out.Domains, s.domainToAPILite(d))
		}
	}
	deployments, err := s.store.ListDeploymentsByProjectID(ctx, out.ID, 1)
	if err != nil {
		return fmt.Errorf("latest deployment: %w", err)
	}
	if len(deployments) == 0 {
		return nil
	}
	latest := deploymentToAPI(deployments[0])
	out.StackKind = latest.StackKind
	out.StackLabel = latest.StackLabel
	if containerRec, err := s.store.GetContainerByDeploymentID(ctx, deployments[0].ID); err == nil {
		c := containerToAPI(containerRec)
		latest.Container = &c
		out.CurrentContainer = &c
	}
	out.LatestDeployment = &latest
	return nil
}

func (s *server) enrichDeploymentsWithContainers(r *http.Request, items []models.Deployment) []apiDeployment {
	if len(items) == 0 {
		return nil
	}
	ctx := r.Context()
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	byDep, err := s.store.GetLatestContainersByDeploymentIDs(ctx, ids)
	if err != nil {
		s.requestLog(r).Warn("batch container lookup failed", "error", err)
		byDep = nil
	}
	out := make([]apiDeployment, 0, len(items))
	for _, d := range items {
		item := deploymentToAPI(d)
		if byDep != nil {
			if containerRec, ok := byDep[d.ID]; ok {
				c := containerToAPI(containerRec)
				item.Container = &c
			}
		}
		out = append(out, item)
	}
	return out
}

func projectToAPI(p models.Project) apiProject {
	rt := strings.TrimSpace(p.DeployRuntime)
	if rt == "" {
		rt = models.DeployRuntimeAuto
	}
	return apiProject{
		ID:                   p.ID,
		Name:                 p.Name,
		RepoURL:              p.RepoURL,
		Branch:               p.Branch,
		GitSource:            p.GitSource,
		GitHubInstallationID: p.GitHubInstallationID,
		Deploy: apiDeployConfig{
			Runtime:    rt,
			InstallCmd: p.DeployInstallCmd,
			BuildCmd:   p.DeployBuildCmd,
			StartCmd:   p.DeployStartCmd,
		},
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
		StackKind:    d.StackKind,
		StackLabel:   d.StackLabel,
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

func (s *server) domainToAPILite(d models.Domain) apiDomain {
	return apiDomain{
		ID:                 d.ID,
		ProjectID:          d.ProjectID,
		DomainName:         d.DomainName,
		SSLStatus:          d.SSLStatus,
		LastCertMessage:    d.LastCertMessage,
		CertCheckedAt:      strings.TrimSpace(d.CertCheckedAtRaw),
		RegistrarDNSStatus: "unknown",
		ResolvedIPv4:       nil,
		CreatedAt:          formatTime(d.CreatedAt),
		UpdatedAt:          formatTime(d.UpdatedAt),
	}
}

func (s *server) domainToAPIExpected(ctx context.Context, d models.Domain, expectedIPv4 string) apiDomain {
	lookupMS := s.cfg.DNSDetectTimeoutMS
	if lookupMS <= 0 {
		lookupMS = 2500
	}
	st, resolved := dnsops.CheckRegistrarARecord(ctx, d.DomainName, expectedIPv4, time.Duration(lookupMS)*time.Millisecond)
	return apiDomain{
		ID:                 d.ID,
		ProjectID:          d.ProjectID,
		DomainName:         d.DomainName,
		SSLStatus:          d.SSLStatus,
		LastCertMessage:    d.LastCertMessage,
		CertCheckedAt:      strings.TrimSpace(d.CertCheckedAtRaw),
		RegistrarDNSStatus: st,
		ResolvedIPv4:       resolved,
		CreatedAt:          formatTime(d.CreatedAt),
		UpdatedAt:          formatTime(d.UpdatedAt),
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
