package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/hostforge/hostforge/internal/models"
)

// UpsertGitHubAppInput sets the singleton github_app row. All *CT fields must already be sealed.
type UpsertGitHubAppInput struct {
	AppID           int64
	Slug            string
	HTMLURL         string
	ClientID        string
	ClientSecretCT  []byte
	PrivateKeyCT    []byte
	WebhookSecretCT []byte
}

// UpsertGitHubApp replaces the singleton github_app row (there is at most one).
func (s *Store) UpsertGitHubApp(ctx context.Context, in UpsertGitHubAppInput) (models.GitHubAppMeta, error) {
	if in.AppID <= 0 {
		return models.GitHubAppMeta{}, fmt.Errorf("app_id must be > 0")
	}
	if len(in.PrivateKeyCT) == 0 {
		return models.GitHubAppMeta{}, fmt.Errorf("private_key_ct required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.GitHubAppMeta{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM github_app WHERE id = 1`); err != nil {
		return models.GitHubAppMeta{}, fmt.Errorf("reset github_app: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO github_app(id, app_id, slug, html_url, client_id, client_secret_ct, private_key_ct, webhook_secret_ct, created_at, updated_at)
		 VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.AppID,
		in.Slug,
		in.HTMLURL,
		in.ClientID,
		in.ClientSecretCT,
		in.PrivateKeyCT,
		in.WebhookSecretCT,
		now,
		now,
	); err != nil {
		return models.GitHubAppMeta{}, fmt.Errorf("insert github_app: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return models.GitHubAppMeta{}, fmt.Errorf("commit github_app: %w", err)
	}
	return s.GetGitHubAppMeta(ctx)
}

// GetGitHubAppMeta returns the singleton App metadata (no secrets).
func (s *Store) GetGitHubAppMeta(ctx context.Context) (models.GitHubAppMeta, error) {
	var m models.GitHubAppMeta
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT app_id, slug, html_url, client_id, created_at, updated_at FROM github_app WHERE id = 1`,
	).Scan(&m.AppID, &m.Slug, &m.HTMLURL, &m.ClientID, &createdAt, &updatedAt)
	if err != nil {
		return models.GitHubAppMeta{}, err
	}
	m.CreatedAt = parseTime(createdAt)
	m.UpdatedAt = parseTime(updatedAt)
	return m, nil
}

// GetGitHubAppSealed returns the sealed App secrets + metadata for token minting and webhook verification.
func (s *Store) GetGitHubAppSealed(ctx context.Context) (models.GitHubAppSecrets, error) {
	var out models.GitHubAppSecrets
	err := s.db.QueryRowContext(
		ctx,
		`SELECT app_id, slug, html_url, client_id, client_secret_ct, private_key_ct, webhook_secret_ct FROM github_app WHERE id = 1`,
	).Scan(&out.AppID, &out.Slug, &out.HTMLURL, &out.ClientID, &out.ClientSecretCT, &out.PrivateKeyCT, &out.WebhookSecretCT)
	if err != nil {
		return models.GitHubAppSecrets{}, err
	}
	return out, nil
}

// DeleteGitHubApp removes the App row (and cascades to installations via DeleteGitHubAppAndInstallations).
func (s *Store) DeleteGitHubApp(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM github_app_installations`); err != nil {
		return fmt.Errorf("delete installations: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM github_app WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("delete github_app: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// UpsertGitHubInstallationInput matches one installation row from the GitHub API.
type UpsertGitHubInstallationInput struct {
	InstallationID int64
	AccountLogin   string
	AccountType    string
	TargetType     string
	RepoSelection  string
	Suspended      bool
}

// UpsertGitHubInstallation inserts or updates one installation row and sets last_synced_at to now.
func (s *Store) UpsertGitHubInstallation(ctx context.Context, in UpsertGitHubInstallationInput) error {
	if in.InstallationID <= 0 {
		return fmt.Errorf("installation_id must be > 0")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	suspended := ""
	if in.Suspended {
		suspended = now
	}
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE github_app_installations
		   SET account_login = ?, account_type = ?, target_type = ?, repo_selection = ?, suspended_at = ?, last_synced_at = ?
		 WHERE installation_id = ?`,
		in.AccountLogin,
		in.AccountType,
		in.TargetType,
		in.RepoSelection,
		suspended,
		now,
		in.InstallationID,
	)
	if err != nil {
		return fmt.Errorf("update installation: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO github_app_installations(installation_id, account_login, account_type, target_type, repo_selection, suspended_at, last_synced_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		in.InstallationID,
		in.AccountLogin,
		in.AccountType,
		in.TargetType,
		in.RepoSelection,
		suspended,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("insert installation: %w", err)
	}
	return nil
}

// GetGitHubInstallation returns one installation row.
func (s *Store) GetGitHubInstallation(ctx context.Context, installationID int64) (models.GitHubInstallation, error) {
	var out models.GitHubInstallation
	var createdAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT installation_id, account_login, account_type, target_type, repo_selection, suspended_at, last_synced_at, created_at FROM github_app_installations WHERE installation_id = ?`,
		installationID,
	).Scan(&out.InstallationID, &out.AccountLogin, &out.AccountType, &out.TargetType, &out.RepoSelection, &out.SuspendedAt, &out.LastSyncedAt, &createdAt)
	if err != nil {
		return models.GitHubInstallation{}, err
	}
	out.CreatedAt = parseTime(createdAt)
	return out, nil
}

// ListGitHubInstallations returns every installation row ordered by account_login.
func (s *Store) ListGitHubInstallations(ctx context.Context) ([]models.GitHubInstallation, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT installation_id, account_login, account_type, target_type, repo_selection, suspended_at, last_synced_at, created_at FROM github_app_installations ORDER BY account_login ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list installations: %w", err)
	}
	defer rows.Close()
	var out []models.GitHubInstallation
	for rows.Next() {
		var item models.GitHubInstallation
		var createdAt string
		if err := rows.Scan(&item.InstallationID, &item.AccountLogin, &item.AccountType, &item.TargetType, &item.RepoSelection, &item.SuspendedAt, &item.LastSyncedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("scan installation: %w", err)
		}
		item.CreatedAt = parseTime(createdAt)
		out = append(out, item)
	}
	return out, rows.Err()
}

// DeleteGitHubInstallation removes one installation row (e.g. when app uninstalled).
func (s *Store) DeleteGitHubInstallation(ctx context.Context, installationID int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM github_app_installations WHERE installation_id = ?`, installationID)
	if err != nil {
		return fmt.Errorf("delete installation: %w", err)
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

// SuspendGitHubInstallation marks an installation as suspended or un-suspended.
func (s *Store) SuspendGitHubInstallation(ctx context.Context, installationID int64, suspended bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	val := ""
	if suspended {
		val = now
	}
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE github_app_installations SET suspended_at = ?, last_synced_at = ? WHERE installation_id = ?`,
		val,
		now,
		installationID,
	)
	if err != nil {
		return fmt.Errorf("suspend installation: %w", err)
	}
	return nil
}

// ErrGitHubAppNotConfigured is returned when operations assume an App row is present.
var ErrGitHubAppNotConfigured = errors.New("github_app_not_configured")
