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

type Application struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Archived    bool      `json:"archived"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Environment struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"application_id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	Kind          string    `json:"kind"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type EnvironmentHealth struct {
	EnvironmentID   string `json:"environment_id"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	ServiceCount    int    `json:"service_count"`
	ConfiguredCount int    `json:"configured_count"`
	RunningCount    int    `json:"running_count"`
	Status          string `json:"status"`
}

type ApplicationSummary struct {
	Application
	EnvironmentHealth   []EnvironmentHealth `json:"environment_health"`
	ServiceCount        int                 `json:"service_count"`
	HealthyServiceCount int                 `json:"healthy_service_count"`
	DomainCount         int                 `json:"domain_count"`
	LatestDeployment    *models.Deployment  `json:"latest_deployment,omitempty"`
}

type Service struct {
	ID                   string    `json:"id"`
	ApplicationID        string    `json:"application_id"`
	Name                 string    `json:"name"`
	RepoURL              string    `json:"repo_url"`
	StackKind            string    `json:"stack_kind,omitempty"`
	StackLabel           string    `json:"stack_label,omitempty"`
	RootDirectory        string    `json:"root_directory"`
	GitHubInstallationID int64     `json:"github_installation_id"`
	DeployRuntime        string    `json:"runtime"`
	InstallCmd           string    `json:"install_cmd"`
	BuildCmd             string    `json:"build_cmd"`
	StartCmd             string    `json:"start_cmd"`
	InternalPort         int       `json:"internal_port"`
	HealthCheckPath      string    `json:"health_check_path"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (s *Store) ListApplications(ctx context.Context) ([]Application, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,description,archived,created_at,updated_at FROM applications ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()
	out := make([]Application, 0)
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

func (s *Store) ListApplicationSummaries(ctx context.Context) ([]ApplicationSummary, error) {
	applications, err := s.ListApplications(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ApplicationSummary, 0, len(applications))
	for _, application := range applications {
		summary := ApplicationSummary{Application: application, EnvironmentHealth: []EnvironmentHealth{}}
		rows, err := s.db.QueryContext(ctx, `
			SELECT e.id,e.name,e.kind,
			       COUNT(se.service_id),
			       COALESCE(SUM(CASE WHEN se.branch<>'' OR se.active_deployment_id<>'' THEN 1 ELSE 0 END),0),
			       COALESCE(SUM(CASE WHEN se.active_deployment_id<>'' AND se.desired_state='running' THEN 1 ELSE 0 END),0)
			FROM environments e
			LEFT JOIN service_environments se ON se.environment_id=e.id
			WHERE e.application_id=?
			GROUP BY e.id,e.name,e.kind
			ORDER BY CASE e.kind WHEN 'production' THEN 0 ELSE 1 END,e.name`, application.ID)
		if err != nil {
			return nil, fmt.Errorf("application environment health: %w", err)
		}
		for rows.Next() {
			var health EnvironmentHealth
			if err := rows.Scan(&health.EnvironmentID, &health.Name, &health.Kind, &health.ServiceCount, &health.ConfiguredCount, &health.RunningCount); err != nil {
				rows.Close()
				return nil, err
			}
			switch {
			case health.ConfiguredCount == 0:
				health.Status = "empty"
			case health.RunningCount == health.ConfiguredCount:
				health.Status = "healthy"
			default:
				health.Status = "degraded"
			}
			summary.EnvironmentHealth = append(summary.EnvironmentHealth, health)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM services WHERE application_id=?`, application.ID).Scan(&summary.ServiceCount); err != nil {
			return nil, err
		}
		for _, health := range summary.EnvironmentHealth {
			if health.RunningCount > summary.HealthyServiceCount {
				summary.HealthyServiceCount = health.RunningCount
			}
		}
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM domains WHERE application_id=?`, application.ID).Scan(&summary.DomainCount); err != nil {
			return nil, err
		}
		latest, err := scanServiceDeployment(s.db.QueryRowContext(ctx, `
			SELECT d.id,d.service_id,d.environment_id,d.status,d.commit_hash,d.logs_path,d.image_ref,d.worktree,d.error_message,d.builder_kind,d.stack_kind,d.stack_label,d.trigger_kind,d.actor,d.cancelled_at,d.rollback_of,d.branch,d.created_at,d.updated_at FROM deployments d
			JOIN services svc ON svc.id=d.service_id
			WHERE svc.application_id=? ORDER BY d.created_at DESC,d.id DESC LIMIT 1`, application.ID))
		if err == nil {
			summary.LatestDeployment = &latest
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, nil
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
	out := make([]Environment, 0)
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT svc.id,svc.application_id,svc.name,svc.repo_url,
		       COALESCE((SELECT d.stack_kind FROM deployments d WHERE d.service_id=svc.id AND (d.stack_kind<>'' OR d.stack_label<>'') ORDER BY d.created_at DESC,d.id DESC LIMIT 1),''),
		       COALESCE((SELECT d.stack_label FROM deployments d WHERE d.service_id=svc.id AND (d.stack_kind<>'' OR d.stack_label<>'') ORDER BY d.created_at DESC,d.id DESC LIMIT 1),''),
		       svc.github_installation_id,svc.root_directory,svc.deploy_runtime,svc.deploy_install_cmd,svc.deploy_build_cmd,svc.deploy_start_cmd,svc.internal_port,svc.health_check_path,svc.created_at,svc.updated_at
		FROM services svc WHERE svc.application_id=? ORDER BY svc.name`, strings.TrimSpace(applicationID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Service, 0)
	for rows.Next() {
		var item Service
		var created, updated string
		if err := rows.Scan(&item.ID, &item.ApplicationID, &item.Name, &item.RepoURL, &item.StackKind, &item.StackLabel, &item.GitHubInstallationID, &item.RootDirectory, &item.DeployRuntime, &item.InstallCmd, &item.BuildCmd, &item.StartCmd, &item.InternalPort, &item.HealthCheckPath, &created, &updated); err != nil {
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

func (s *Store) UpdateApplication(ctx context.Context, id, name, description string, archived *bool) (Application, error) {
	item, err := s.GetApplication(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Application{}, ErrApplicationNotFound
	}
	if err != nil {
		return Application{}, err
	}
	if strings.TrimSpace(name) != "" {
		item.Name = strings.TrimSpace(name)
	}
	item.Description = strings.TrimSpace(description)
	if archived != nil {
		item.Archived = *archived
	}
	archivedValue := 0
	if item.Archived {
		archivedValue = 1
	}
	_, err = s.db.ExecContext(ctx, `UPDATE applications SET name=?,description=?,archived=?,updated_at=? WHERE id=?`, item.Name, item.Description, archivedValue, time.Now().UTC().Format(time.RFC3339), item.ID)
	if err != nil {
		return Application{}, err
	}
	return s.GetApplication(ctx, item.ID)
}

func (s *Store) DeleteApplication(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM applications WHERE id=?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrApplicationNotFound
	}
	return nil
}
