package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/models"
)

const gitAuthProviderGitHub = "github"

// GetProjectGitAuthMeta returns metadata for one project's stored git auth.
func (s *Store) GetProjectGitAuthMeta(ctx context.Context, projectID string) (models.ProjectGitAuthMeta, error) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return models.ProjectGitAuthMeta{}, fmt.Errorf("empty project id")
	}
	var m models.ProjectGitAuthMeta
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT project_id, provider, token_last4, created_at, updated_at FROM project_git_auth WHERE project_id = ?`,
		pid,
	).Scan(&m.ProjectID, &m.Provider, &m.TokenLast4, &createdAt, &updatedAt)
	if err != nil {
		return models.ProjectGitAuthMeta{}, err
	}
	m.CreatedAt = parseTime(createdAt)
	m.UpdatedAt = parseTime(updatedAt)
	return m, nil
}

// GetProjectGitAuthSealed returns ciphertext for one project's git auth.
func (s *Store) GetProjectGitAuthSealed(ctx context.Context, projectID string) (models.ProjectGitAuthSealed, error) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return models.ProjectGitAuthSealed{}, fmt.Errorf("empty project id")
	}
	var out models.ProjectGitAuthSealed
	err := s.db.QueryRowContext(
		ctx,
		`SELECT project_id, provider, token_ct FROM project_git_auth WHERE project_id = ?`,
		pid,
	).Scan(&out.ProjectID, &out.Provider, &out.TokenCT)
	if err != nil {
		return models.ProjectGitAuthSealed{}, err
	}
	return out, nil
}

// UpsertProjectGitHubAuth creates or updates GitHub token ciphertext for a project.
func (s *Store) UpsertProjectGitHubAuth(ctx context.Context, projectID string, tokenCT []byte, tokenLast4 string) (models.ProjectGitAuthMeta, error) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return models.ProjectGitAuthMeta{}, fmt.Errorf("empty project id")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE project_git_auth
		   SET provider = ?, token_ct = ?, token_last4 = ?, updated_at = ?
		 WHERE project_id = ?`,
		gitAuthProviderGitHub,
		tokenCT,
		tokenLast4,
		now,
		pid,
	)
	if err != nil {
		return models.ProjectGitAuthMeta{}, fmt.Errorf("update project git auth: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = s.db.ExecContext(
			ctx,
			`INSERT INTO project_git_auth(project_id, provider, token_ct, token_last4, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			pid,
			gitAuthProviderGitHub,
			tokenCT,
			tokenLast4,
			now,
			now,
		)
		if err != nil {
			return models.ProjectGitAuthMeta{}, fmt.Errorf("insert project git auth: %w", err)
		}
	}
	return s.GetProjectGitAuthMeta(ctx, pid)
}

// DeleteProjectGitAuth removes a project's git auth row.
func (s *Store) DeleteProjectGitAuth(ctx context.Context, projectID string) error {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return fmt.Errorf("empty project id")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM project_git_auth WHERE project_id = ?`, pid)
	if err != nil {
		return fmt.Errorf("delete project git auth: %w", err)
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
