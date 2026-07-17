package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrApplicationNotFound  = errors.New("application_not_found")
	ErrEnvironmentNotFound  = errors.New("environment_not_found")
	ErrServiceNotFound      = errors.New("service_not_found")
	ErrDuplicateService     = errors.New("duplicate_service")
	ErrDuplicateEnvironment = errors.New("duplicate_environment")
)

type ServiceEnvironment struct {
	ServiceID          string    `json:"service_id"`
	EnvironmentID      string    `json:"environment_id"`
	Branch             string    `json:"branch"`
	ActiveDeploymentID string    `json:"active_deployment_id"`
	DesiredState       string    `json:"desired_state"`
	AutoDeploy         bool      `json:"auto_deploy"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CreateServiceInput struct {
	ApplicationID, Name, RepoURL, RootDirectory   string
	GitHubInstallationID                          int64
	DeployRuntime, InstallCmd, BuildCmd, StartCmd string
	InternalPort                                  int
	HealthCheckPath                               string
	InitialEnvironmentID                          string
	InitialBranch                                 string
	InitialAutoDeploy                             bool
}

func (s *Store) GetService(ctx context.Context, id string) (Service, error) {
	var item Service
	var created, updated string
	err := s.db.QueryRowContext(ctx, `
		SELECT svc.id,svc.application_id,svc.service_type,svc.name,svc.repo_url,
		       COALESCE((SELECT d.stack_kind FROM deployments d WHERE d.service_id=svc.id AND (d.stack_kind<>'' OR d.stack_label<>'') ORDER BY d.created_at DESC,d.id DESC LIMIT 1),''),
		       COALESCE((SELECT d.stack_label FROM deployments d WHERE d.service_id=svc.id AND (d.stack_kind<>'' OR d.stack_label<>'') ORDER BY d.created_at DESC,d.id DESC LIMIT 1),''),
		       svc.github_installation_id,svc.root_directory,svc.deploy_runtime,svc.deploy_install_cmd,svc.deploy_build_cmd,svc.deploy_start_cmd,svc.internal_port,svc.health_check_path,svc.created_at,svc.updated_at
		FROM services svc WHERE svc.id=?`, strings.TrimSpace(id)).Scan(&item.ID, &item.ApplicationID, &item.ServiceType, &item.Name, &item.RepoURL, &item.StackKind, &item.StackLabel, &item.GitHubInstallationID, &item.RootDirectory, &item.DeployRuntime, &item.InstallCmd, &item.BuildCmd, &item.StartCmd, &item.InternalPort, &item.HealthCheckPath, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Service{}, ErrServiceNotFound
	}
	if err != nil {
		return Service{}, err
	}
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

func (s *Store) CreateService(ctx context.Context, in CreateServiceInput) (Service, error) {
	if _, err := s.GetApplication(ctx, in.ApplicationID); errors.Is(err, sql.ErrNoRows) {
		return Service{}, ErrApplicationNotFound
	} else if err != nil {
		return Service{}, err
	}
	now := time.Now().UTC()
	if in.InternalPort == 0 {
		in.InternalPort = 3000
	}
	if strings.TrimSpace(in.HealthCheckPath) == "" {
		in.HealthCheckPath = "/"
	}
	if strings.TrimSpace(in.DeployRuntime) == "" {
		in.DeployRuntime = "auto"
	}
	item := Service{ID: newID(), ApplicationID: strings.TrimSpace(in.ApplicationID), ServiceType: "application", Name: strings.TrimSpace(in.Name), RepoURL: strings.TrimSpace(in.RepoURL), GitHubInstallationID: in.GitHubInstallationID, RootDirectory: strings.TrimSpace(in.RootDirectory), DeployRuntime: strings.TrimSpace(in.DeployRuntime), InstallCmd: strings.TrimSpace(in.InstallCmd), BuildCmd: strings.TrimSpace(in.BuildCmd), StartCmd: strings.TrimSpace(in.StartCmd), InternalPort: in.InternalPort, HealthCheckPath: strings.TrimSpace(in.HealthCheckPath), CreatedAt: now, UpdatedAt: now}
	if item.Name == "" || item.RepoURL == "" {
		return Service{}, fmt.Errorf("name and repository required")
	}
	stamp := now.Format(time.RFC3339)
	envs, err := s.ListApplicationEnvironments(ctx, item.ApplicationID)
	if err != nil {
		return Service{}, err
	}
	initialEnvironmentID := strings.TrimSpace(in.InitialEnvironmentID)
	initialBranch := strings.TrimSpace(in.InitialBranch)
	if initialEnvironmentID != "" {
		found := false
		for _, env := range envs {
			if env.ID == initialEnvironmentID {
				found = true
				break
			}
		}
		if !found {
			return Service{}, ErrEnvironmentNotFound
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Service{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO services(id,application_id,service_type,name,repo_url,github_installation_id,root_directory,deploy_runtime,deploy_install_cmd,deploy_build_cmd,deploy_start_cmd,internal_port,health_check_path,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.ApplicationID, item.ServiceType, item.Name, item.RepoURL, item.GitHubInstallationID, item.RootDirectory, item.DeployRuntime, item.InstallCmd, item.BuildCmd, item.StartCmd, item.InternalPort, item.HealthCheckPath, stamp, stamp)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Service{}, ErrDuplicateService
		}
		return Service{}, err
	}
	for _, env := range envs {
		branch := ""
		autoDeploy := 0
		if env.ID == initialEnvironmentID {
			branch = initialBranch
			if in.InitialAutoDeploy {
				autoDeploy = 1
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO service_environments(service_id,environment_id,branch,auto_deploy,created_at,updated_at) VALUES(?,?,?,?,?,?)`, item.ID, env.ID, branch, autoDeploy, stamp, stamp); err != nil {
			return Service{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Service{}, err
	}
	return item, nil
}

func (s *Store) UpdateService(ctx context.Context, id string, in CreateServiceInput) (Service, error) {
	current, err := s.GetService(ctx, id)
	if err != nil {
		return Service{}, err
	}
	if strings.TrimSpace(in.Name) != "" {
		current.Name = strings.TrimSpace(in.Name)
	}
	if strings.TrimSpace(in.RepoURL) != "" {
		current.RepoURL = strings.TrimSpace(in.RepoURL)
	}
	current.RootDirectory = strings.TrimSpace(in.RootDirectory)
	current.GitHubInstallationID = in.GitHubInstallationID
	if in.InternalPort > 0 {
		current.InternalPort = in.InternalPort
	}
	if strings.TrimSpace(in.HealthCheckPath) != "" {
		current.HealthCheckPath = strings.TrimSpace(in.HealthCheckPath)
	}
	if strings.TrimSpace(in.DeployRuntime) != "" {
		current.DeployRuntime = strings.TrimSpace(in.DeployRuntime)
	}
	current.InstallCmd = strings.TrimSpace(in.InstallCmd)
	current.BuildCmd = strings.TrimSpace(in.BuildCmd)
	current.StartCmd = strings.TrimSpace(in.StartCmd)
	_, err = s.db.ExecContext(ctx, `UPDATE services SET name=?,repo_url=?,github_installation_id=?,root_directory=?,deploy_runtime=?,deploy_install_cmd=?,deploy_build_cmd=?,deploy_start_cmd=?,internal_port=?,health_check_path=?,updated_at=? WHERE id=?`, current.Name, current.RepoURL, current.GitHubInstallationID, current.RootDirectory, current.DeployRuntime, current.InstallCmd, current.BuildCmd, current.StartCmd, current.InternalPort, current.HealthCheckPath, time.Now().UTC().Format(time.RFC3339), current.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Service{}, ErrDuplicateService
		}
		return Service{}, err
	}
	return s.GetService(ctx, current.ID)
}

func (s *Store) DeleteService(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM services WHERE id=?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

func (s *Store) GetEnvironment(ctx context.Context, id string) (Environment, error) {
	var item Environment
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,application_id,name,slug,kind,created_at,updated_at FROM environments WHERE id=?`, strings.TrimSpace(id)).Scan(&item.ID, &item.ApplicationID, &item.Name, &item.Slug, &item.Kind, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Environment{}, ErrEnvironmentNotFound
	}
	if err != nil {
		return Environment{}, err
	}
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

func (s *Store) UpdateEnvironment(ctx context.Context, id, name string) (Environment, error) {
	item, err := s.GetEnvironment(ctx, id)
	if err != nil {
		return Environment{}, err
	}
	if strings.TrimSpace(name) != "" {
		item.Name = strings.TrimSpace(name)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE environments SET name=?,updated_at=? WHERE id=?`, item.Name, time.Now().UTC().Format(time.RFC3339), item.ID)
	if err != nil {
		return Environment{}, err
	}
	return s.GetEnvironment(ctx, item.ID)
}

func (s *Store) CreateEnvironment(ctx context.Context, applicationID, name, slug, kind string) (Environment, error) {
	applicationID = strings.TrimSpace(applicationID)
	name = strings.TrimSpace(name)
	slug = strings.ToLower(strings.TrimSpace(slug))
	kind = strings.ToLower(strings.TrimSpace(kind))
	if _, err := s.GetApplication(ctx, applicationID); errors.Is(err, sql.ErrNoRows) {
		return Environment{}, ErrApplicationNotFound
	} else if err != nil {
		return Environment{}, err
	}
	if name == "" || slug == "" || (kind != "production" && kind != "staging") {
		return Environment{}, fmt.Errorf("invalid environment")
	}
	for _, char := range slug {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return Environment{}, fmt.Errorf("invalid environment slug")
		}
	}
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339)
	item := Environment{ID: newID(), ApplicationID: applicationID, Name: name, Slug: slug, Kind: kind, CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Environment{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO environments(id,application_id,name,slug,kind,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, item.ID, item.ApplicationID, item.Name, item.Slug, item.Kind, stamp, stamp); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Environment{}, ErrDuplicateEnvironment
		}
		return Environment{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO service_environments(service_id,environment_id,branch,auto_deploy,active_deployment_id,desired_state,created_at,updated_at)
		SELECT id,?,'',0,'','running',?,? FROM services WHERE application_id=? AND service_type='application'`, item.ID, stamp, stamp, applicationID); err != nil {
		return Environment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Environment{}, err
	}
	return item, nil
}

func (s *Store) ListServiceEnvironments(ctx context.Context, serviceID string) ([]ServiceEnvironment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT service_id,environment_id,branch,auto_deploy,active_deployment_id,desired_state,created_at,updated_at FROM service_environments WHERE service_id=? ORDER BY environment_id`, strings.TrimSpace(serviceID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ServiceEnvironment, 0)
	for rows.Next() {
		var item ServiceEnvironment
		var auto int
		var created, updated string
		if err := rows.Scan(&item.ServiceID, &item.EnvironmentID, &item.Branch, &auto, &item.ActiveDeploymentID, &item.DesiredState, &created, &updated); err != nil {
			return nil, err
		}
		item.AutoDeploy = auto != 0
		item.CreatedAt = parseTime(created)
		item.UpdatedAt = parseTime(updated)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetServiceEnvironment(ctx context.Context, serviceID, environmentID string) (ServiceEnvironment, error) {
	var item ServiceEnvironment
	var auto int
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT service_id,environment_id,branch,auto_deploy,active_deployment_id,desired_state,created_at,updated_at FROM service_environments WHERE service_id=? AND environment_id=?`, serviceID, environmentID).Scan(&item.ServiceID, &item.EnvironmentID, &item.Branch, &auto, &item.ActiveDeploymentID, &item.DesiredState, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ServiceEnvironment{}, ErrEnvironmentNotFound
	}
	if err != nil {
		return ServiceEnvironment{}, err
	}
	item.AutoDeploy = auto != 0
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

func (s *Store) UpdateServiceEnvironment(ctx context.Context, serviceID, environmentID, branch string, autoDeploy bool) (ServiceEnvironment, error) {
	auto := 0
	if autoDeploy {
		auto = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE service_environments SET branch=?,auto_deploy=?,updated_at=? WHERE service_id=? AND environment_id=?`, strings.TrimSpace(branch), auto, time.Now().UTC().Format(time.RFC3339), serviceID, environmentID)
	if err != nil {
		return ServiceEnvironment{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ServiceEnvironment{}, ErrEnvironmentNotFound
	}
	return s.GetServiceEnvironment(ctx, serviceID, environmentID)
}
