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
