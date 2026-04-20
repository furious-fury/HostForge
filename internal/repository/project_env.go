package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/models"
)

const maxProjectEnvVarsPerProject = 100

// ErrProjectEnvLimitExceeded is returned when a project already has MaxEnvVarsPerProject rows.
var ErrProjectEnvLimitExceeded = errors.New("project_env_limit_exceeded")

// SealedEnvVar is one env key sealed for storage (no plaintext).
type SealedEnvVar struct {
	Key        string
	ValueCT    []byte
	ValueLast4 string
}

// CreateProjectWithSealedEnv inserts a project and optional env rows in one transaction.
func (s *Store) CreateProjectWithSealedEnv(ctx context.Context, in CreateProjectInput, env []SealedEnvVar) (models.Project, error) {
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
	p := models.Project{
		ID:               newID(),
		Name:             name,
		RepoURL:          repoURL,
		Branch:           branch,
		DeployRuntime:    rt,
		DeployInstallCmd: strings.TrimSpace(in.DeployInstallCmd),
		DeployBuildCmd:   strings.TrimSpace(in.DeployBuildCmd),
		DeployStartCmd:   strings.TrimSpace(in.DeployStartCmd),
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Project{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO projects(id, name, repo_url, branch, deploy_runtime, deploy_install_cmd, deploy_build_cmd, deploy_start_cmd, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID,
		p.Name,
		p.RepoURL,
		p.Branch,
		p.DeployRuntime,
		p.DeployInstallCmd,
		p.DeployBuildCmd,
		p.DeployStartCmd,
		p.CreatedAt.Format(time.RFC3339),
		p.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return models.Project{}, fmt.Errorf("insert project: %w", err)
	}

	for _, e := range env {
		id := newID()
		ts := time.Now().UTC().Format(time.RFC3339)
		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO project_env_vars(id, project_id, key, value_ct, value_last4, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id,
			p.ID,
			e.Key,
			e.ValueCT,
			e.ValueLast4,
			ts,
			ts,
		)
		if err != nil {
			return models.Project{}, fmt.Errorf("insert project env: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return models.Project{}, fmt.Errorf("commit: %w", err)
	}
	return p, nil
}

// ListProjectEnvMeta returns env rows without ciphertext.
func (s *Store) ListProjectEnvMeta(ctx context.Context, projectID string) ([]models.ProjectEnvVar, error) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return nil, fmt.Errorf("empty project id")
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, project_id, key, value_last4, created_at, updated_at FROM project_env_vars WHERE project_id = ? ORDER BY key ASC`,
		pid,
	)
	if err != nil {
		return nil, fmt.Errorf("list project env: %w", err)
	}
	defer rows.Close()

	var out []models.ProjectEnvVar
	for rows.Next() {
		var e models.ProjectEnvVar
		var createdAt, updatedAt string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Key, &e.ValueLast4, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan project env: %w", err)
		}
		e.CreatedAt = parseTime(createdAt)
		e.UpdatedAt = parseTime(updatedAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListProjectEnvSealed returns key + ciphertext for runtime injection.
func (s *Store) ListProjectEnvSealed(ctx context.Context, projectID string) ([]models.ProjectEnvSealed, error) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return nil, fmt.Errorf("empty project id")
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT key, value_ct FROM project_env_vars WHERE project_id = ? ORDER BY key ASC`,
		pid,
	)
	if err != nil {
		return nil, fmt.Errorf("list project env sealed: %w", err)
	}
	defer rows.Close()

	var out []models.ProjectEnvSealed
	for rows.Next() {
		var e models.ProjectEnvSealed
		if err := rows.Scan(&e.Key, &e.ValueCT); err != nil {
			return nil, fmt.Errorf("scan sealed env: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountProjectEnvVars returns how many env rows exist for a project.
func (s *Store) CountProjectEnvVars(ctx context.Context, projectID string) (int, error) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return 0, fmt.Errorf("empty project id")
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_env_vars WHERE project_id = ?`, pid).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count project env: %w", err)
	}
	return n, nil
}

// UpsertProjectEnvVar inserts or updates by (project_id, key).
func (s *Store) UpsertProjectEnvVar(ctx context.Context, projectID string, key string, valueCT []byte, valueLast4 string) (models.ProjectEnvVar, error) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return models.ProjectEnvVar{}, fmt.Errorf("empty project id")
	}
	k := strings.TrimSpace(key)
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := s.db.ExecContext(
		ctx,
		`UPDATE project_env_vars SET value_ct = ?, value_last4 = ?, updated_at = ? WHERE project_id = ? AND key = ?`,
		valueCT,
		valueLast4,
		now,
		pid,
		k,
	)
	if err != nil {
		return models.ProjectEnvVar{}, fmt.Errorf("update project env: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return s.getProjectEnvMetaByProjectAndKey(ctx, pid, k)
	}

	var cnt int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_env_vars WHERE project_id = ?`, pid).Scan(&cnt); err != nil {
		return models.ProjectEnvVar{}, fmt.Errorf("count project env: %w", err)
	}
	if cnt >= maxProjectEnvVarsPerProject {
		return models.ProjectEnvVar{}, ErrProjectEnvLimitExceeded
	}

	id := newID()
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO project_env_vars(id, project_id, key, value_ct, value_last4, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id,
		pid,
		k,
		valueCT,
		valueLast4,
		now,
		now,
	)
	if err != nil {
		return models.ProjectEnvVar{}, fmt.Errorf("insert project env: %w", err)
	}
	return s.getProjectEnvMetaByProjectAndKey(ctx, pid, k)
}

func (s *Store) getProjectEnvMetaByProjectAndKey(ctx context.Context, projectID, key string) (models.ProjectEnvVar, error) {
	var e models.ProjectEnvVar
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, key, value_last4, created_at, updated_at FROM project_env_vars WHERE project_id = ? AND key = ?`,
		projectID,
		key,
	).Scan(&e.ID, &e.ProjectID, &e.Key, &e.ValueLast4, &createdAt, &updatedAt)
	if err != nil {
		return models.ProjectEnvVar{}, err
	}
	e.CreatedAt = parseTime(createdAt)
	e.UpdatedAt = parseTime(updatedAt)
	return e, nil
}

// GetProjectEnvMetaByID returns metadata if the row belongs to projectID.
func (s *Store) GetProjectEnvMetaByID(ctx context.Context, projectID, envID string) (models.ProjectEnvVar, error) {
	pid := strings.TrimSpace(projectID)
	eid := strings.TrimSpace(envID)
	var e models.ProjectEnvVar
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, key, value_last4, created_at, updated_at FROM project_env_vars WHERE id = ? AND project_id = ?`,
		eid,
		pid,
	).Scan(&e.ID, &e.ProjectID, &e.Key, &e.ValueLast4, &createdAt, &updatedAt)
	if err != nil {
		return models.ProjectEnvVar{}, err
	}
	e.CreatedAt = parseTime(createdAt)
	e.UpdatedAt = parseTime(updatedAt)
	return e, nil
}

// UpdateProjectEnvValue updates ciphertext for an existing row by id.
func (s *Store) UpdateProjectEnvValue(ctx context.Context, projectID, envID string, valueCT []byte, valueLast4 string) (models.ProjectEnvVar, error) {
	pid := strings.TrimSpace(projectID)
	eid := strings.TrimSpace(envID)
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE project_env_vars SET value_ct = ?, value_last4 = ?, updated_at = ? WHERE id = ? AND project_id = ?`,
		valueCT,
		valueLast4,
		now,
		eid,
		pid,
	)
	if err != nil {
		return models.ProjectEnvVar{}, fmt.Errorf("update env value: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return models.ProjectEnvVar{}, err
	}
	if n == 0 {
		return models.ProjectEnvVar{}, sql.ErrNoRows
	}
	return s.GetProjectEnvMetaByID(ctx, pid, eid)
}

// DeleteProjectEnvVar removes one env row.
func (s *Store) DeleteProjectEnvVar(ctx context.Context, projectID, envID string) error {
	pid := strings.TrimSpace(projectID)
	eid := strings.TrimSpace(envID)
	res, err := s.db.ExecContext(ctx, `DELETE FROM project_env_vars WHERE id = ? AND project_id = ?`, eid, pid)
	if err != nil {
		return fmt.Errorf("delete project env: %w", err)
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
