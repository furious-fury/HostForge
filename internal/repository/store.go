package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/models"
)

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

// GetProjectByRepoAndBranch returns an existing project by repo URL and branch.
func (s *Store) GetProjectByRepoAndBranch(ctx context.Context, repoURL, branch string) (models.Project, error) {
	trimmedRepo := strings.TrimSpace(repoURL)
	trimmedBranch := strings.TrimSpace(branch)
	var p models.Project
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, repo_url, branch, created_at, updated_at FROM projects WHERE repo_url = ? AND branch = ?`,
		trimmedRepo,
		trimmedBranch,
	).Scan(&p.ID, &p.Name, &p.RepoURL, &p.Branch, &createdAt, &updatedAt)
	if err != nil {
		return models.Project{}, fmt.Errorf("lookup project by repo+branch: %w", err)
	}
	p.CreatedAt = parseTime(createdAt)
	p.UpdatedAt = parseTime(updatedAt)
	return p, nil
}

// ListProjectsByRepoURL returns projects that share repo_url, newest first.
func (s *Store) ListProjectsByRepoURL(ctx context.Context, repoURL string) ([]models.Project, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, name, repo_url, branch, created_at, updated_at FROM projects WHERE repo_url = ? ORDER BY created_at DESC`,
		strings.TrimSpace(repoURL),
	)
	if err != nil {
		return nil, fmt.Errorf("list projects by repo: %w", err)
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.RepoURL, &p.Branch, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		p.CreatedAt = parseTime(createdAt)
		p.UpdatedAt = parseTime(updatedAt)
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// EnsureProject returns the project for repoURL and branch, inserting a row if missing.
// Branch is part of the unique key with repo_url (default branch stored as "").
func (s *Store) EnsureProject(ctx context.Context, repoURL, branch string) (models.Project, error) {
	trimmedRepo := strings.TrimSpace(repoURL)
	trimmedBranch := strings.TrimSpace(branch)

	var p models.Project
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, repo_url, branch, created_at, updated_at FROM projects WHERE repo_url = ? AND branch = ?`,
		trimmedRepo,
		trimmedBranch,
	).Scan(&p.ID, &p.Name, &p.RepoURL, &p.Branch, &createdAt, &updatedAt)
	if err == nil {
		p.CreatedAt = parseTime(createdAt)
		p.UpdatedAt = parseTime(updatedAt)
		return p, nil
	}
	if err != sql.ErrNoRows {
		return models.Project{}, fmt.Errorf("lookup project: %w", err)
	}

	now := time.Now().UTC()
	p = models.Project{
		ID:        newID(),
		Name:      projectNameFromURL(trimmedRepo),
		RepoURL:   trimmedRepo,
		Branch:    trimmedBranch,
		CreatedAt: now,
		UpdatedAt: now,
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
		`INSERT INTO deployments(id, project_id, status, commit_hash, logs_path, image_ref, worktree, error_message, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID,
		d.ProjectID,
		d.Status,
		d.CommitHash,
		d.LogsPath,
		d.ImageRef,
		d.Worktree,
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

// GetLatestSuccessfulDeploymentByProjectID returns the newest SUCCESS deployment for a project.
func (s *Store) GetLatestSuccessfulDeploymentByProjectID(ctx context.Context, projectID string) (models.Deployment, error) {
	var d models.Deployment
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, status, commit_hash, logs_path, image_ref, worktree, error_message, created_at, updated_at
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, repo_url, branch, created_at, updated_at FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var items []models.Project
	for rows.Next() {
		var p models.Project
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.RepoURL, &p.Branch, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		p.CreatedAt = parseTime(createdAt)
		p.UpdatedAt = parseTime(updatedAt)
		items = append(items, p)
	}
	return items, rows.Err()
}

// ListDeployments returns all deployments, newest first by created_at.
func (s *Store) ListDeployments(ctx context.Context) ([]models.Deployment, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, project_id, status, commit_hash, logs_path, image_ref, worktree, error_message, created_at, updated_at
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
		`INSERT INTO domains(id, project_id, domain_name, ssl_status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		d.ID,
		d.ProjectID,
		d.DomainName,
		d.SSLStatus,
		d.CreatedAt.Format(time.RFC3339),
		d.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return models.Domain{}, fmt.Errorf("insert domain: %w", err)
	}
	return d, nil
}

// ListDomainsByProject returns all domains for projectID.
func (s *Store) ListDomainsByProject(ctx context.Context, projectID string) ([]models.Domain, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, project_id, domain_name, ssl_status, created_at, updated_at FROM domains WHERE project_id = ? ORDER BY domain_name ASC`,
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
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.DomainName, &d.SSLStatus, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan domain: %w", err)
		}
		d.CreatedAt = parseTime(createdAt)
		d.UpdatedAt = parseTime(updatedAt)
		out = append(out, d)
	}
	return out, rows.Err()
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
