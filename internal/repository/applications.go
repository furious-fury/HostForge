package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Application struct {
	ID, Name, Description string
	Archived              bool
	CreatedAt, UpdatedAt  time.Time
}

type Environment struct {
	ID, ApplicationID, Name, Slug, Kind string
	CreatedAt, UpdatedAt                time.Time
}

type Service struct {
	ID, ApplicationID, Name, RepoURL, RootDirectory string
	GitHubInstallationID                            int64
	DeployRuntime, InstallCmd, BuildCmd, StartCmd   string
	InternalPort                                    int
	HealthCheckPath                                 string
	CreatedAt, UpdatedAt                            time.Time
}

func (s *Store) ListApplications(ctx context.Context) ([]Application, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,description,archived,created_at,updated_at FROM applications ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()
	var out []Application
	for rows.Next() {
		var item Application
		var archived int
		var created, updated string
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &archived, &created, &updated); err != nil {
			return nil, err
		}
		item.Archived = archived != 0
		item.CreatedAt = parseTime(created)
		item.UpdatedAt = parseTime(updated)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetApplication(ctx context.Context, id string) (Application, error) {
	var item Application
	var archived int
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,name,description,archived,created_at,updated_at FROM applications WHERE id=?`, strings.TrimSpace(id)).Scan(&item.ID, &item.Name, &item.Description, &archived, &created, &updated)
	if err != nil {
		return Application{}, err
	}
	item.Archived = archived != 0
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

func (s *Store) ListApplicationEnvironments(ctx context.Context, applicationID string) ([]Environment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,application_id,name,slug,kind,created_at,updated_at FROM environments WHERE application_id=? ORDER BY kind`, strings.TrimSpace(applicationID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Environment
	for rows.Next() {
		var item Environment
		var created, updated string
		if err := rows.Scan(&item.ID, &item.ApplicationID, &item.Name, &item.Slug, &item.Kind, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(created)
		item.UpdatedAt = parseTime(updated)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListApplicationServices(ctx context.Context, applicationID string) ([]Service, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,application_id,name,repo_url,github_installation_id,root_directory,deploy_runtime,deploy_install_cmd,deploy_build_cmd,deploy_start_cmd,internal_port,health_check_path,created_at,updated_at FROM services WHERE application_id=? ORDER BY name`, strings.TrimSpace(applicationID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Service
	for rows.Next() {
		var item Service
		var created, updated string
		if err := rows.Scan(&item.ID, &item.ApplicationID, &item.Name, &item.RepoURL, &item.GitHubInstallationID, &item.RootDirectory, &item.DeployRuntime, &item.InstallCmd, &item.BuildCmd, &item.StartCmd, &item.InternalPort, &item.HealthCheckPath, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(created)
		item.UpdatedAt = parseTime(updated)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreateApplication(ctx context.Context, name, description string) (Application, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Application{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	item := Application{ID: newID(), Name: strings.TrimSpace(name), Description: strings.TrimSpace(description), CreatedAt: now, UpdatedAt: now}
	if item.Name == "" {
		return Application{}, fmt.Errorf("application name required")
	}
	stamp := now.Format(time.RFC3339)
	if _, err = tx.ExecContext(ctx, `INSERT INTO applications(id,name,description,archived,created_at,updated_at) VALUES(?,?,?,0,?,?)`, item.ID, item.Name, item.Description, stamp, stamp); err != nil {
		return Application{}, err
	}
	for _, env := range []struct{ name, slug, kind string }{{"Production", "production", "production"}, {"Staging", "staging", "staging"}} {
		if _, err = tx.ExecContext(ctx, `INSERT INTO environments(id,application_id,name,slug,kind,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, newID(), item.ID, env.name, env.slug, env.kind, stamp, stamp); err != nil {
			return Application{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Application{}, err
	}
	return item, nil
}

var _ = sql.ErrNoRows
