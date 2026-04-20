package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/models"
)

// UpsertProjectSSHKeyInput contains the sealed ed25519 keypair for a project.
type UpsertProjectSSHKeyInput struct {
	ProjectID    string
	PublicKey    string
	PrivateKeyCT []byte
	Fingerprint  string
}

// UpsertProjectSSHKey creates or replaces the SSH deploy key for a project.
func (s *Store) UpsertProjectSSHKey(ctx context.Context, in UpsertProjectSSHKeyInput) (models.ProjectSSHKeyMeta, error) {
	pid := strings.TrimSpace(in.ProjectID)
	if pid == "" {
		return models.ProjectSSHKeyMeta{}, fmt.Errorf("empty project id")
	}
	if strings.TrimSpace(in.PublicKey) == "" {
		return models.ProjectSSHKeyMeta{}, fmt.Errorf("empty public key")
	}
	if len(in.PrivateKeyCT) == 0 {
		return models.ProjectSSHKeyMeta{}, fmt.Errorf("empty sealed private key")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ProjectSSHKeyMeta{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_ssh_keys WHERE project_id = ?`, pid); err != nil {
		return models.ProjectSSHKeyMeta{}, fmt.Errorf("reset project ssh key: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO project_ssh_keys(project_id, public_key, private_key_ct, fingerprint, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		pid,
		strings.TrimSpace(in.PublicKey),
		in.PrivateKeyCT,
		strings.TrimSpace(in.Fingerprint),
		now,
	); err != nil {
		return models.ProjectSSHKeyMeta{}, fmt.Errorf("insert project ssh key: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return models.ProjectSSHKeyMeta{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetProjectSSHKeyMeta(ctx, pid)
}

// GetProjectSSHKeyMeta returns public key metadata for a project.
func (s *Store) GetProjectSSHKeyMeta(ctx context.Context, projectID string) (models.ProjectSSHKeyMeta, error) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return models.ProjectSSHKeyMeta{}, fmt.Errorf("empty project id")
	}
	var m models.ProjectSSHKeyMeta
	var createdAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT project_id, public_key, fingerprint, created_at FROM project_ssh_keys WHERE project_id = ?`,
		pid,
	).Scan(&m.ProjectID, &m.PublicKey, &m.Fingerprint, &createdAt)
	if err != nil {
		return models.ProjectSSHKeyMeta{}, err
	}
	m.CreatedAt = parseTime(createdAt)
	return m, nil
}

// GetProjectSSHKeySealed returns sealed private key for deploy-time transport auth.
func (s *Store) GetProjectSSHKeySealed(ctx context.Context, projectID string) (models.ProjectSSHKeySealed, error) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return models.ProjectSSHKeySealed{}, fmt.Errorf("empty project id")
	}
	var out models.ProjectSSHKeySealed
	err := s.db.QueryRowContext(
		ctx,
		`SELECT project_id, public_key, private_key_ct FROM project_ssh_keys WHERE project_id = ?`,
		pid,
	).Scan(&out.ProjectID, &out.PublicKey, &out.PrivateKeyCT)
	if err != nil {
		return models.ProjectSSHKeySealed{}, err
	}
	return out, nil
}

// DeleteProjectSSHKey removes a project's SSH key row.
func (s *Store) DeleteProjectSSHKey(ctx context.Context, projectID string) error {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return fmt.Errorf("empty project id")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM project_ssh_keys WHERE project_id = ?`, pid)
	if err != nil {
		return fmt.Errorf("delete project ssh key: %w", err)
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
