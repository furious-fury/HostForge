package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/models"
)

// ErrDuplicateDomain is returned when inserting or renaming to an existing domain_name.
var ErrDuplicateDomain = errors.New("duplicate_domain")

// ErrDomainNotFound is returned when no domain row matches the requested id.
var ErrDomainNotFound = errors.New("domain_not_found")

// Store wraps database/sql access for HostForge persistence.
type Store struct {
	db *sql.DB
}

// New returns a Store that uses db (typically from database.OpenSQLite).
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// CreateDeploymentInput holds optional metadata for a new deployment row.
type CreateDeploymentInput struct {
	ProjectID  string
	CommitHash string
	LogsPath   string
	ImageRef   string
	Worktree   string
}

// AttachContainerInput links a running Docker container to a deployment.
type AttachContainerInput struct {
	DeploymentID      string
	DockerContainerID string
	InternalPort      int
	HostPort          int
	Status            string
}

// CreateProjectInput defines explicit project fields supplied by the UI/API.
type CreateProjectInput struct {
	Name             string
	RepoURL          string
	Branch           string
	DeployRuntime    string
	DeployInstallCmd string
	DeployBuildCmd   string
	DeployStartCmd   string
	// GitSource (url | github_app | ssh). Empty defaults to "url".
	GitSource string
	// GitHubInstallationID is used when GitSource=github_app.
	GitHubInstallationID int64
}

// projectSelectColumns lists columns returned by every SELECT against projects, in scan order.
const projectSelectColumns = `id, name, repo_url, branch, deploy_runtime, deploy_install_cmd, deploy_build_cmd, deploy_start_cmd, git_source, github_installation_id, created_at, updated_at`

// scanProject scans a row produced by a `SELECT `+projectSelectColumns+` FROM projects ...` query.
func scanProject(row interface {
	Scan(dest ...any) error
}) (models.Project, error) {
	var p models.Project
	var createdAt, updatedAt string
	if err := row.Scan(
		&p.ID,
		&p.Name,
		&p.RepoURL,
		&p.Branch,
		&p.DeployRuntime,
		&p.DeployInstallCmd,
		&p.DeployBuildCmd,
		&p.DeployStartCmd,
		&p.GitSource,
		&p.GitHubInstallationID,
		&createdAt,
		&updatedAt,
	); err != nil {
		return models.Project{}, err
	}
	if strings.TrimSpace(p.GitSource) == "" {
		p.GitSource = models.GitSourceURL
	}
	p.CreatedAt = parseTime(createdAt)
	p.UpdatedAt = parseTime(updatedAt)
	return p, nil
}

// GetProjectByRepoAndBranch returns an existing project by repo URL and branch.
func (s *Store) GetProjectByRepoAndBranch(ctx context.Context, repoURL, branch string) (models.Project, error) {
	trimmedRepo := strings.TrimSpace(repoURL)
	trimmedBranch := strings.TrimSpace(branch)
	row := s.db.QueryRowContext(
		ctx,
		`SELECT `+projectSelectColumns+` FROM projects WHERE repo_url = ? AND branch = ?`,
		trimmedRepo,
		trimmedBranch,
	)
	p, err := scanProject(row)
	if err != nil {
		return models.Project{}, fmt.Errorf("lookup project by repo+branch: %w", err)
	}
	return p, nil
}

// ListProjectsByRepoURL returns projects that share repo_url, newest first.
func (s *Store) ListProjectsByRepoURL(ctx context.Context, repoURL string) ([]models.Project, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT `+projectSelectColumns+` FROM projects WHERE repo_url = ? ORDER BY created_at DESC`,
		strings.TrimSpace(repoURL),
	)
	if err != nil {
		return nil, fmt.Errorf("list projects by repo: %w", err)
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// EnsureProject returns the project for repoURL and branch, inserting a row if missing.
// Branch is part of the unique key with repo_url (default branch stored as "").
func (s *Store) EnsureProject(ctx context.Context, repoURL, branch string) (models.Project, error) {
	trimmedRepo := strings.TrimSpace(repoURL)
	trimmedBranch := strings.TrimSpace(branch)

	row := s.db.QueryRowContext(
		ctx,
		`SELECT `+projectSelectColumns+` FROM projects WHERE repo_url = ? AND branch = ?`,
		trimmedRepo,
		trimmedBranch,
	)
	p, err := scanProject(row)
	if err == nil {
		return p, nil
	}
	if err != sql.ErrNoRows {
		return models.Project{}, fmt.Errorf("lookup project: %w", err)
	}

	now := time.Now().UTC()
	p = models.Project{
		ID:               newID(),
		Name:             projectNameFromURL(trimmedRepo),
		RepoURL:          trimmedRepo,
		Branch:           trimmedBranch,
		DeployRuntime:    models.DeployRuntimeAuto,
		DeployInstallCmd: "",
		DeployBuildCmd:   "",
		DeployStartCmd:   "",
		GitSource:        models.GitSourceURL,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO projects(id, name, repo_url, branch, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID,
		p.Name,
		p.RepoURL,
		p.Branch,
		p.CreatedAt.Format(time.RFC3339),
		p.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return models.Project{}, fmt.Errorf("insert project: %w", err)
	}
	return p, nil
}

// CreateProject inserts a new project row with explicit fields.
func (s *Store) CreateProject(ctx context.Context, in CreateProjectInput) (models.Project, error) {
	now := time.Now().UTC()
	repoURL := strings.TrimSpace(in.RepoURL)
	branch := strings.TrimSpace(in.Branch)
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = projectNameFromURL(repoURL)
	}
	rt := strings.TrimSpace(in.DeployRuntime)
	if rt == "" {
		rt = models.DeployRuntimeAuto
	}
	gs := strings.TrimSpace(in.GitSource)
	if gs == "" {
		gs = models.GitSourceURL
	}
	p := models.Project{
		ID:                   newID(),
		Name:                 name,
		RepoURL:              repoURL,
		Branch:               branch,
		DeployRuntime:        rt,
		DeployInstallCmd:     strings.TrimSpace(in.DeployInstallCmd),
		DeployBuildCmd:       strings.TrimSpace(in.DeployBuildCmd),
		DeployStartCmd:       strings.TrimSpace(in.DeployStartCmd),
		GitSource:            gs,
		GitHubInstallationID: in.GitHubInstallationID,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO projects(id, name, repo_url, branch, deploy_runtime, deploy_install_cmd, deploy_build_cmd, deploy_start_cmd, git_source, github_installation_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID,
		p.Name,
		p.RepoURL,
		p.Branch,
		p.DeployRuntime,
		p.DeployInstallCmd,
		p.DeployBuildCmd,
		p.DeployStartCmd,
		p.GitSource,
		p.GitHubInstallationID,
		p.CreatedAt.Format(time.RFC3339),
		p.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return models.Project{}, fmt.Errorf("insert project: %w", err)
	}
	return p, nil
}

// UpdateProjectGitSource sets git_source and github_installation_id for a project.
// gitSource must be one of models.GitSourceURL, GitSourceGitHubApp, GitSourceSSH.
func (s *Store) UpdateProjectGitSource(ctx context.Context, projectID, gitSource string, installationID int64) error {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return fmt.Errorf("empty project id")
	}
	gs := strings.TrimSpace(gitSource)
	switch gs {
	case models.GitSourceURL, models.GitSourceGitHubApp, models.GitSourceSSH:
	default:
		return fmt.Errorf("invalid git_source %q", gitSource)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE projects SET git_source = ?, github_installation_id = ?, updated_at = ? WHERE id = ?`,
		gs,
		installationID,
		now,
		pid,
	)
	if err != nil {
		return fmt.Errorf("update project git_source: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CreateDeployment inserts a deployment with status QUEUED.
func (s *Store) CreateDeployment(ctx context.Context, in CreateDeploymentInput) (models.Deployment, error) {
	now := time.Now().UTC()
	d := models.Deployment{
		ID:         newID(),
		ProjectID:  in.ProjectID,
		Status:     models.DeploymentQueued,
		CommitHash: strings.TrimSpace(in.CommitHash),
		LogsPath:   strings.TrimSpace(in.LogsPath),
		ImageRef:   strings.TrimSpace(in.ImageRef),
		Worktree:   strings.TrimSpace(in.Worktree),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO deployments(id, project_id, status, commit_hash, logs_path, image_ref, worktree, error_message, stack_kind, stack_label, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID,
		d.ProjectID,
		d.Status,
		d.CommitHash,
		d.LogsPath,
		d.ImageRef,
		d.Worktree,
		"",
		"",
		"",
		d.CreatedAt.Format(time.RFC3339),
		d.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return models.Deployment{}, fmt.Errorf("insert deployment: %w", err)
	}
	return d, nil
}

// UpdateDeploymentStatus sets status and optional error_message (terminal failures).
func (s *Store) UpdateDeploymentStatus(ctx context.Context, deploymentID, status, errorMessage string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE deployments SET status = ?, error_message = ?, updated_at = ? WHERE id = ?`,
		status,
		strings.TrimSpace(errorMessage),
		now,
		deploymentID,
	)
	if err != nil {
		return fmt.Errorf("update deployment status: %w", err)
	}
	return nil
}

// UpdateDeploymentLogsPath sets logs_path for a deployment.
func (s *Store) UpdateDeploymentLogsPath(ctx context.Context, deploymentID, logsPath string) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE deployments SET logs_path = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(logsPath),
		time.Now().UTC().Format(time.RFC3339),
		strings.TrimSpace(deploymentID),
	)
	if err != nil {
		return fmt.Errorf("update deployment logs_path: %w", err)
	}
	return nil
}

// UpdateDeploymentStack sets stack_kind and stack_label from nixpacks plan summary (deploy pipeline).
func (s *Store) UpdateDeploymentStack(ctx context.Context, deploymentID, stackKind, stackLabel string) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE deployments SET stack_kind = ?, stack_label = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(stackKind),
		strings.TrimSpace(stackLabel),
		time.Now().UTC().Format(time.RFC3339),
		strings.TrimSpace(deploymentID),
	)
	if err != nil {
		return fmt.Errorf("update deployment stack: %w", err)
	}
	return nil
}

// GetLatestSuccessfulDeploymentByProjectID returns the newest SUCCESS deployment for a project.
func (s *Store) GetLatestSuccessfulDeploymentByProjectID(ctx context.Context, projectID string) (models.Deployment, error) {
	var d models.Deployment
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, status, commit_hash, logs_path, image_ref, worktree, error_message, stack_kind, stack_label, created_at, updated_at
		 FROM deployments
		 WHERE project_id = ? AND status = ?
		 ORDER BY created_at DESC
		 LIMIT 1`,
		strings.TrimSpace(projectID),
		models.DeploymentSuccess,
	).Scan(
		&d.ID,
		&d.ProjectID,
		&d.Status,
		&d.CommitHash,
		&d.LogsPath,
		&d.ImageRef,
		&d.Worktree,
		&d.ErrorMessage,
		&d.StackKind,
		&d.StackLabel,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return models.Deployment{}, fmt.Errorf("lookup latest successful deployment: %w", err)
	}
	d.CreatedAt = parseTime(createdAt)
	d.UpdatedAt = parseTime(updatedAt)
	return d, nil
}

// GetPreviousSuccessfulDeploymentByProjectID returns the second newest SUCCESS deployment for a project.
func (s *Store) GetPreviousSuccessfulDeploymentByProjectID(ctx context.Context, projectID string) (models.Deployment, error) {
	var d models.Deployment
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, status, commit_hash, logs_path, image_ref, worktree, error_message, stack_kind, stack_label, created_at, updated_at
		 FROM deployments
		 WHERE project_id = ? AND status = ?
		 ORDER BY created_at DESC
		 LIMIT 1 OFFSET 1`,
		strings.TrimSpace(projectID),
		models.DeploymentSuccess,
	).Scan(
		&d.ID,
		&d.ProjectID,
		&d.Status,
		&d.CommitHash,
		&d.LogsPath,
		&d.ImageRef,
		&d.Worktree,
		&d.ErrorMessage,
		&d.StackKind,
		&d.StackLabel,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return models.Deployment{}, fmt.Errorf("lookup previous successful deployment: %w", err)
	}
	d.CreatedAt = parseTime(createdAt)
	d.UpdatedAt = parseTime(updatedAt)
	return d, nil
}

// GetDeploymentByID returns a deployment row by id.
func (s *Store) GetDeploymentByID(ctx context.Context, deploymentID string) (models.Deployment, error) {
	var d models.Deployment
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, status, commit_hash, logs_path, image_ref, worktree, error_message, stack_kind, stack_label, created_at, updated_at
		 FROM deployments
		 WHERE id = ?`,
		strings.TrimSpace(deploymentID),
	).Scan(
		&d.ID,
		&d.ProjectID,
		&d.Status,
		&d.CommitHash,
		&d.LogsPath,
		&d.ImageRef,
		&d.Worktree,
		&d.ErrorMessage,
		&d.StackKind,
		&d.StackLabel,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return models.Deployment{}, fmt.Errorf("lookup deployment by id: %w", err)
	}
	d.CreatedAt = parseTime(createdAt)
	d.UpdatedAt = parseTime(updatedAt)
	return d, nil
}

// DeleteProjectCascade removes all rows for projectID: containers (via deployments),
// deployments, domains, then the project. It runs in a single transaction.
func (s *Store) DeleteProjectCascade(ctx context.Context, projectID string) error {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return fmt.Errorf("project id must not be empty")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(
		ctx,
		`DELETE FROM containers WHERE deployment_id IN (SELECT id FROM deployments WHERE project_id = ?)`,
		pid,
	); err != nil {
		return fmt.Errorf("delete containers: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM deployments WHERE project_id = ?`, pid); err != nil {
		return fmt.Errorf("delete deployments: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM domains WHERE project_id = ?`, pid); err != nil {
		return fmt.Errorf("delete domains: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_git_auth WHERE project_id = ?`, pid); err != nil {
		return fmt.Errorf("delete project git auth: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_ssh_keys WHERE project_id = ?`, pid); err != nil {
		return fmt.Errorf("delete project ssh keys: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_env_vars WHERE project_id = ?`, pid); err != nil {
		return fmt.Errorf("delete project env vars: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, pid); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// GetProjectByID returns a project row by id.
func (s *Store) GetProjectByID(ctx context.Context, projectID string) (models.Project, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT `+projectSelectColumns+` FROM projects WHERE id = ?`,
		strings.TrimSpace(projectID),
	)
	p, err := scanProject(row)
	if err != nil {
		return models.Project{}, fmt.Errorf("lookup project by id: %w", err)
	}
	return p, nil
}

// UpdateProjectDeployConfig updates Nixpacks-related deploy fields for a project.
func (s *Store) UpdateProjectDeployConfig(ctx context.Context, projectID, runtime, install, build, start string) (models.Project, error) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return models.Project{}, fmt.Errorf("empty project id")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE projects SET deploy_runtime = ?, deploy_install_cmd = ?, deploy_build_cmd = ?, deploy_start_cmd = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(runtime),
		strings.TrimSpace(install),
		strings.TrimSpace(build),
		strings.TrimSpace(start),
		now,
		pid,
	)
	if err != nil {
		return models.Project{}, fmt.Errorf("update project deploy config: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return models.Project{}, err
	}
	if n == 0 {
		return models.Project{}, sql.ErrNoRows
	}
	return s.GetProjectByID(ctx, pid)
}

// AttachContainer inserts a container row for a successful run (ports and Docker ID).
func (s *Store) AttachContainer(ctx context.Context, in AttachContainerInput) (models.Container, error) {
	now := time.Now().UTC()
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "RUNNING"
	}
	c := models.Container{
		ID:                newID(),
		DeploymentID:      in.DeploymentID,
		DockerContainerID: strings.TrimSpace(in.DockerContainerID),
		InternalPort:      in.InternalPort,
		HostPort:          in.HostPort,
		Status:            status,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO containers(id, deployment_id, docker_container_id, internal_port, host_port, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID,
		c.DeploymentID,
		c.DockerContainerID,
		c.InternalPort,
		c.HostPort,
		c.Status,
		c.CreatedAt.Format(time.RFC3339),
		c.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return models.Container{}, fmt.Errorf("insert container: %w", err)
	}
	return c, nil
}

// GetContainerByDeploymentID returns the container row linked to deploymentID.
func (s *Store) GetContainerByDeploymentID(ctx context.Context, deploymentID string) (models.Container, error) {
	var c models.Container
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, deployment_id, docker_container_id, internal_port, host_port, status, created_at, updated_at
		 FROM containers
		 WHERE deployment_id = ?
		 ORDER BY created_at DESC
		 LIMIT 1`,
		strings.TrimSpace(deploymentID),
	).Scan(
		&c.ID,
		&c.DeploymentID,
		&c.DockerContainerID,
		&c.InternalPort,
		&c.HostPort,
		&c.Status,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return models.Container{}, fmt.Errorf("lookup container by deployment: %w", err)
	}
	c.CreatedAt = parseTime(createdAt)
	c.UpdatedAt = parseTime(updatedAt)
	return c, nil
}

// ListAllocatedHostPorts returns host ports currently claimed by any non-REMOVED container.
// excludeContainerID is optional; pass "" to include every active container. The returned
// map is keyed by host_port (>0 only), suitable for fast membership checks during port
// allocation so concurrent projects don't reuse the same published port.
func (s *Store) ListAllocatedHostPorts(ctx context.Context, excludeContainerID string) (map[int]struct{}, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, host_port FROM containers WHERE status != 'REMOVED' AND host_port > 0`,
	)
	if err != nil {
		return nil, fmt.Errorf("list allocated host ports: %w", err)
	}
	defer rows.Close()
	exclude := strings.TrimSpace(excludeContainerID)
	out := make(map[int]struct{})
	for rows.Next() {
		var id string
		var port int
		if err := rows.Scan(&id, &port); err != nil {
			return nil, fmt.Errorf("scan allocated host port: %w", err)
		}
		if exclude != "" && id == exclude {
			continue
		}
		out[port] = struct{}{}
	}
	return out, rows.Err()
}

// UpdateContainerHostBinding updates docker_container_id and host_port for a container row.
// Used when a container is recreated (e.g. on restart after its port was stolen).
func (s *Store) UpdateContainerHostBinding(ctx context.Context, containerID, dockerContainerID string, hostPort int, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE containers SET docker_container_id = ?, host_port = ?, status = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(dockerContainerID),
		hostPort,
		strings.TrimSpace(status),
		now,
		strings.TrimSpace(containerID),
	)
	if err != nil {
		return fmt.Errorf("update container host binding: %w", err)
	}
	return nil
}

// UpdateContainerStatus updates a container row status and updated_at timestamp.
func (s *Store) UpdateContainerStatus(ctx context.Context, containerID, status string) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE containers SET status = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(status),
		time.Now().UTC().Format(time.RFC3339),
		strings.TrimSpace(containerID),
	)
	if err != nil {
		return fmt.Errorf("update container status: %w", err)
	}
	return nil
}

// ListProjects returns all projects, newest first by created_at.
func (s *Store) ListProjects(ctx context.Context) ([]models.Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+projectSelectColumns+` FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var items []models.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

// ListDeployments returns all deployments, newest first by created_at.
func (s *Store) ListDeployments(ctx context.Context) ([]models.Deployment, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, project_id, status, commit_hash, logs_path, image_ref, worktree, error_message, stack_kind, stack_label, created_at, updated_at
		 FROM deployments ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	defer rows.Close()

	var items []models.Deployment
	for rows.Next() {
		var d models.Deployment
		var createdAt, updatedAt string
		if err := rows.Scan(
			&d.ID,
			&d.ProjectID,
			&d.Status,
			&d.CommitHash,
			&d.LogsPath,
			&d.ImageRef,
			&d.Worktree,
			&d.ErrorMessage,
			&d.StackKind,
			&d.StackLabel,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		d.CreatedAt = parseTime(createdAt)
		d.UpdatedAt = parseTime(updatedAt)
		items = append(items, d)
	}
	return items, rows.Err()
}

// ListDeploymentsRecent returns the newest deployments across all projects, capped at limit (1..500).
func (s *Store) ListDeploymentsRecent(ctx context.Context, limit int) ([]models.Deployment, error) {
	lim := limit
	if lim <= 0 || lim > 500 {
		lim = 100
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, project_id, status, commit_hash, logs_path, image_ref, worktree, error_message, stack_kind, stack_label, created_at, updated_at
		 FROM deployments ORDER BY created_at DESC LIMIT ?`,
		lim,
	)
	if err != nil {
		return nil, fmt.Errorf("list deployments recent: %w", err)
	}
	defer rows.Close()

	var items []models.Deployment
	for rows.Next() {
		var d models.Deployment
		var createdAt, updatedAt string
		if err := rows.Scan(
			&d.ID,
			&d.ProjectID,
			&d.Status,
			&d.CommitHash,
			&d.LogsPath,
			&d.ImageRef,
			&d.Worktree,
			&d.ErrorMessage,
			&d.StackKind,
			&d.StackLabel,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		d.CreatedAt = parseTime(createdAt)
		d.UpdatedAt = parseTime(updatedAt)
		items = append(items, d)
	}
	return items, rows.Err()
}

// ListDeploymentsWithEmptyStack returns deployments that have no nixpacks stack summary yet (stack_kind and stack_label both empty).
// Newest first. lim caps rows (default 500 when caller passes <=0).
func (s *Store) ListDeploymentsWithEmptyStack(ctx context.Context, lim int) ([]models.Deployment, error) {
	if lim <= 0 || lim > 5000 {
		lim = 500
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, project_id, status, commit_hash, logs_path, image_ref, worktree, error_message, stack_kind, stack_label, created_at, updated_at
		 FROM deployments
		 WHERE stack_kind = '' AND stack_label = ''
		 ORDER BY created_at DESC
		 LIMIT ?`,
		lim,
	)
	if err != nil {
		return nil, fmt.Errorf("list deployments with empty stack: %w", err)
	}
	defer rows.Close()

	var items []models.Deployment
	for rows.Next() {
		var d models.Deployment
		var createdAt, updatedAt string
		if err := rows.Scan(
			&d.ID,
			&d.ProjectID,
			&d.Status,
			&d.CommitHash,
			&d.LogsPath,
			&d.ImageRef,
			&d.Worktree,
			&d.ErrorMessage,
			&d.StackKind,
			&d.StackLabel,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		d.CreatedAt = parseTime(createdAt)
		d.UpdatedAt = parseTime(updatedAt)
		items = append(items, d)
	}
	return items, rows.Err()
}

// GetLatestContainersByDeploymentIDs returns the newest container row per deployment_id for the given IDs.
func (s *Store) GetLatestContainersByDeploymentIDs(ctx context.Context, deploymentIDs []string) (map[string]models.Container, error) {
	out := make(map[string]models.Container)
	if len(deploymentIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(deploymentIDs))
	ph := make([]string, 0, len(deploymentIDs))
	for _, id := range deploymentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		args = append(args, id)
		ph = append(ph, "?")
	}
	if len(args) == 0 {
		return out, nil
	}
	inClause := strings.Join(ph, ",")
	q := fmt.Sprintf(`
SELECT id, deployment_id, docker_container_id, internal_port, host_port, status, created_at, updated_at
FROM (
  SELECT id, deployment_id, docker_container_id, internal_port, host_port, status, created_at, updated_at,
    ROW_NUMBER() OVER (PARTITION BY deployment_id ORDER BY created_at DESC) AS rn
  FROM containers
  WHERE deployment_id IN (%s)
) WHERE rn = 1`, inClause)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("batch latest containers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c models.Container
		var createdAt, updatedAt string
		if err := rows.Scan(
			&c.ID,
			&c.DeploymentID,
			&c.DockerContainerID,
			&c.InternalPort,
			&c.HostPort,
			&c.Status,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan container batch: %w", err)
		}
		c.CreatedAt = parseTime(createdAt)
		c.UpdatedAt = parseTime(updatedAt)
		out[c.DeploymentID] = c
	}
	return out, rows.Err()
}

// ListDeploymentsByProjectID returns deployments for one project, newest first.
func (s *Store) ListDeploymentsByProjectID(ctx context.Context, projectID string, limit int) ([]models.Deployment, error) {
	lim := limit
	if lim <= 0 || lim > 500 {
		lim = 100
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, project_id, status, commit_hash, logs_path, image_ref, worktree, error_message, stack_kind, stack_label, created_at, updated_at
		 FROM deployments
		 WHERE project_id = ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		strings.TrimSpace(projectID),
		lim,
	)
	if err != nil {
		return nil, fmt.Errorf("list deployments by project: %w", err)
	}
	defer rows.Close()

	var items []models.Deployment
	for rows.Next() {
		var d models.Deployment
		var createdAt, updatedAt string
		if err := rows.Scan(
			&d.ID,
			&d.ProjectID,
			&d.Status,
			&d.CommitHash,
			&d.LogsPath,
			&d.ImageRef,
			&d.Worktree,
			&d.ErrorMessage,
			&d.StackKind,
			&d.StackLabel,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		d.CreatedAt = parseTime(createdAt)
		d.UpdatedAt = parseTime(updatedAt)
		items = append(items, d)
	}
	return items, rows.Err()
}

// CreateDomain inserts a domain record for a project.
func (s *Store) CreateDomain(ctx context.Context, projectID, domainName string) (models.Domain, error) {
	now := time.Now().UTC()
	d := models.Domain{
		ID:         newID(),
		ProjectID:  strings.TrimSpace(projectID),
		DomainName: strings.TrimSpace(domainName),
		SSLStatus:  models.SSLStatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO domains(id, project_id, domain_name, ssl_status, last_cert_message, cert_checked_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID,
		d.ProjectID,
		d.DomainName,
		d.SSLStatus,
		"",
		"",
		d.CreatedAt.Format(time.RFC3339),
		d.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return models.Domain{}, ErrDuplicateDomain
		}
		return models.Domain{}, fmt.Errorf("insert domain: %w", err)
	}
	return d, nil
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") && strings.Contains(msg, "constraint")
}

// GetDomainByProjectAndName returns a domain for a project by hostname (exact match).
func (s *Store) GetDomainByProjectAndName(ctx context.Context, projectID, domainName string) (models.Domain, error) {
	pid := strings.TrimSpace(projectID)
	name := strings.TrimSpace(domainName)
	if pid == "" || name == "" {
		return models.Domain{}, fmt.Errorf("missing project id or domain name")
	}
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, domain_name, ssl_status, last_cert_message, cert_checked_at, created_at, updated_at FROM domains WHERE project_id = ? AND domain_name = ?`,
		pid,
		name,
	)
	var d models.Domain
	var createdAt, updatedAt string
	if err := row.Scan(&d.ID, &d.ProjectID, &d.DomainName, &d.SSLStatus, &d.LastCertMessage, &d.CertCheckedAtRaw, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Domain{}, ErrDomainNotFound
		}
		return models.Domain{}, fmt.Errorf("get domain by name: %w", err)
	}
	d.CreatedAt = parseTime(createdAt)
	d.UpdatedAt = parseTime(updatedAt)
	return d, nil
}

// GetDomainByID returns a domain row by primary key.
func (s *Store) GetDomainByID(ctx context.Context, domainID string) (models.Domain, error) {
	id := strings.TrimSpace(domainID)
	if id == "" {
		return models.Domain{}, fmt.Errorf("empty domain id")
	}
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, domain_name, ssl_status, last_cert_message, cert_checked_at, created_at, updated_at FROM domains WHERE id = ?`,
		id,
	)
	var d models.Domain
	var createdAt, updatedAt string
	if err := row.Scan(&d.ID, &d.ProjectID, &d.DomainName, &d.SSLStatus, &d.LastCertMessage, &d.CertCheckedAtRaw, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Domain{}, ErrDomainNotFound
		}
		return models.Domain{}, fmt.Errorf("get domain: %w", err)
	}
	d.CreatedAt = parseTime(createdAt)
	d.UpdatedAt = parseTime(updatedAt)
	return d, nil
}

// DeleteDomain removes a domain row by id. Returns ErrDomainNotFound if no row was deleted.
func (s *Store) DeleteDomain(ctx context.Context, domainID string) error {
	id := strings.TrimSpace(domainID)
	if id == "" {
		return fmt.Errorf("empty domain id")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM domains WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete domain: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrDomainNotFound
	}
	return nil
}

// UpdateDomainName changes the hostname for a domain owned by projectID. Resets ssl_status to PENDING.
func (s *Store) UpdateDomainName(ctx context.Context, projectID, domainID, newName string) (models.Domain, error) {
	pid := strings.TrimSpace(projectID)
	did := strings.TrimSpace(domainID)
	name := strings.TrimSpace(newName)
	if pid == "" || did == "" || name == "" {
		return models.Domain{}, fmt.Errorf("missing project id, domain id, or name")
	}
	existing, err := s.GetDomainByID(ctx, did)
	if err != nil {
		return models.Domain{}, err
	}
	if existing.ProjectID != pid {
		return models.Domain{}, ErrDomainNotFound
	}
	var conflict int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM domains WHERE domain_name = ? AND id != ?`,
		name,
		did,
	).Scan(&conflict); err != nil {
		return models.Domain{}, fmt.Errorf("check domain name: %w", err)
	}
	if conflict > 0 {
		return models.Domain{}, ErrDuplicateDomain
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE domains SET domain_name = ?, ssl_status = ?, last_cert_message = '', cert_checked_at = '', updated_at = ? WHERE id = ? AND project_id = ?`,
		name,
		models.SSLStatusPending,
		now,
		did,
		pid,
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return models.Domain{}, ErrDuplicateDomain
		}
		return models.Domain{}, fmt.Errorf("update domain: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return models.Domain{}, err
	}
	if n == 0 {
		return models.Domain{}, ErrDomainNotFound
	}
	return s.GetDomainByID(ctx, did)
}

// ListDomainsByProject returns all domains for projectID.
func (s *Store) ListDomainsByProject(ctx context.Context, projectID string) ([]models.Domain, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, project_id, domain_name, ssl_status, last_cert_message, cert_checked_at, created_at, updated_at FROM domains WHERE project_id = ? ORDER BY domain_name ASC`,
		strings.TrimSpace(projectID),
	)
	if err != nil {
		return nil, fmt.Errorf("list domains by project: %w", err)
	}
	defer rows.Close()
	var out []models.Domain
	for rows.Next() {
		var d models.Domain
		var createdAt, updatedAt string
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.DomainName, &d.SSLStatus, &d.LastCertMessage, &d.CertCheckedAtRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan domain: %w", err)
		}
		d.CreatedAt = parseTime(createdAt)
		d.UpdatedAt = parseTime(updatedAt)
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListAllDomains returns every registered domain row (for background jobs).
func (s *Store) ListAllDomains(ctx context.Context) ([]models.Domain, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, project_id, domain_name, ssl_status, last_cert_message, cert_checked_at, created_at, updated_at FROM domains ORDER BY domain_name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all domains: %w", err)
	}
	defer rows.Close()
	var out []models.Domain
	for rows.Next() {
		var d models.Domain
		var createdAt, updatedAt string
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.DomainName, &d.SSLStatus, &d.LastCertMessage, &d.CertCheckedAtRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan domain: %w", err)
		}
		d.CreatedAt = parseTime(createdAt)
		d.UpdatedAt = parseTime(updatedAt)
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateDomainCertObservation sets optional cert poll fields (does not change ssl_status).
func (s *Store) UpdateDomainCertObservation(ctx context.Context, domainID, message string, checkedAt time.Time) error {
	id := strings.TrimSpace(domainID)
	if id == "" {
		return fmt.Errorf("empty domain id")
	}
	msg := strings.TrimSpace(message)
	checked := ""
	if !checkedAt.IsZero() {
		checked = checkedAt.UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE domains SET last_cert_message = ?, cert_checked_at = ? WHERE id = ?`,
		msg,
		checked,
		id,
	)
	if err != nil {
		return fmt.Errorf("update domain cert observation: %w", err)
	}
	return nil
}

// UpdateDomainSSLStatus updates ssl_status for a domain.
func (s *Store) UpdateDomainSSLStatus(ctx context.Context, domainID, status string) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE domains SET ssl_status = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(status),
		time.Now().UTC().Format(time.RFC3339),
		strings.TrimSpace(domainID),
	)
	if err != nil {
		return fmt.Errorf("update domain ssl_status: %w", err)
	}
	return nil
}

// ListDomainRoutes resolves domains to latest successful deployment host port per project.
func (s *Store) ListDomainRoutes(ctx context.Context) ([]models.DomainRoute, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
	d.id,
	d.project_id,
	d.domain_name,
	d.ssl_status,
	d.last_cert_message,
	d.cert_checked_at,
	d.created_at,
	d.updated_at,
	c.host_port
FROM domains d
LEFT JOIN deployments dep ON dep.id = (
	SELECT dep2.id
	FROM deployments dep2
	WHERE dep2.project_id = d.project_id AND dep2.status = 'SUCCESS'
	ORDER BY dep2.created_at DESC
	LIMIT 1
)
LEFT JOIN containers c ON c.deployment_id = dep.id
ORDER BY d.domain_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list domain routes: %w", err)
	}
	defer rows.Close()

	var routes []models.DomainRoute
	for rows.Next() {
		var route models.DomainRoute
		var createdAt, updatedAt string
		var hostPort sql.NullInt64
		if err := rows.Scan(
			&route.ID,
			&route.ProjectID,
			&route.DomainName,
			&route.SSLStatus,
			&route.LastCertMessage,
			&route.CertCheckedAtRaw,
			&createdAt,
			&updatedAt,
			&hostPort,
		); err != nil {
			return nil, fmt.Errorf("scan domain route: %w", err)
		}
		route.CreatedAt = parseTime(createdAt)
		route.UpdatedAt = parseTime(updatedAt)
		if hostPort.Valid {
			route.HostPort = int(hostPort.Int64)
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

// projectNameFromURL derives a display name from the repo path (e.g. "myapp" from github.com/org/myapp).
func projectNameFromURL(repoURL string) string {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "project"
	}
	base := strings.TrimSuffix(path.Base(strings.TrimSuffix(u.Path, "/")), ".git")
	if base == "." || base == "/" || base == "" {
		return "project"
	}
	return base
}

// parseTime parses RFC3339 timestamps stored in SQLite TEXT columns.
func parseTime(raw string) time.Time {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

// newID returns a 32-hex-character identifier (128 random bits, or time-based fallback).
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(b)
}
