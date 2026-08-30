package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/models"
)

var (
	ErrDuplicateDomain                  = errors.New("duplicate_domain")
	ErrDomainNotFound                   = errors.New("domain_not_found")
	ErrManagedDomain                    = errors.New("managed_domain")
	ErrEnvironmentVariableLimitExceeded = errors.New("environment_variable_limit_exceeded")
)

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

type AttachContainerInput struct {
	DeploymentID      string
	DockerContainerID string
	InternalPort      int
	HostPort          int
	Status            string
}

func (s *Store) UpdateDeploymentStatus(ctx context.Context, deploymentID, status, errorMessage string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE deployments SET status=?,error_message=?,updated_at=? WHERE id=?`, status, strings.TrimSpace(errorMessage), now, strings.TrimSpace(deploymentID))
	if err != nil {
		return fmt.Errorf("update deployment status: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO platform_events(application_id,service_id,environment_id,deployment_id,event_type,status,actor,message,detail,created_at)
		SELECT COALESCE(svc.application_id,''),d.service_id,d.environment_id,d.id,
		       'deployment',?,COALESCE(d.actor,''),'Deployment ' || lower(?),?,?
		FROM deployments d JOIN services svc ON svc.id=d.service_id WHERE d.id=?`,
		status, status, strings.TrimSpace(errorMessage), now, strings.TrimSpace(deploymentID))
	if err != nil {
		return fmt.Errorf("record deployment status event: %w", err)
	}
	return tx.Commit()
}

func (s *Store) UpdateDeploymentCommitHash(ctx context.Context, deploymentID, commitHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET commit_hash=?,updated_at=? WHERE id=?`, strings.TrimSpace(commitHash), time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(deploymentID))
	return err
}

func (s *Store) UpdateDeploymentLogsPath(ctx context.Context, deploymentID, logsPath string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET logs_path=?,updated_at=? WHERE id=?`, strings.TrimSpace(logsPath), time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(deploymentID))
	return err
}

func (s *Store) UpdateDeploymentStack(ctx context.Context, deploymentID, stackKind, stackLabel string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET stack_kind=?,stack_label=?,updated_at=? WHERE id=?`, strings.TrimSpace(stackKind), strings.TrimSpace(stackLabel), time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(deploymentID))
	return err
}

func (s *Store) UpdateDeploymentBuilder(ctx context.Context, deploymentID, builderKind, stackKind, stackLabel string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET builder_kind=?,stack_kind=?,stack_label=?,updated_at=? WHERE id=?`, strings.TrimSpace(builderKind), strings.TrimSpace(stackKind), strings.TrimSpace(stackLabel), time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(deploymentID))
	return err
}

func (s *Store) GetDeploymentByID(ctx context.Context, deploymentID string) (models.Deployment, error) {
	return s.GetServiceDeployment(ctx, deploymentID)
}

func (s *Store) AttachContainer(ctx context.Context, in AttachContainerInput) (models.Container, error) {
	now := time.Now().UTC()
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "RUNNING"
	}
	item := models.Container{ID: newID(), DeploymentID: strings.TrimSpace(in.DeploymentID), DockerContainerID: strings.TrimSpace(in.DockerContainerID), InternalPort: in.InternalPort, HostPort: in.HostPort, Status: status, CreatedAt: now, UpdatedAt: now}
	_, err := s.db.ExecContext(ctx, `INSERT INTO containers(id,deployment_id,docker_container_id,internal_port,host_port,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		item.ID, item.DeploymentID, item.DockerContainerID, item.InternalPort, item.HostPort, item.Status, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return models.Container{}, fmt.Errorf("insert container: %w", err)
	}
	return item, nil
}

func (s *Store) GetContainerByDeploymentID(ctx context.Context, deploymentID string) (models.Container, error) {
	var item models.Container
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT id,deployment_id,docker_container_id,internal_port,host_port,status,created_at,updated_at FROM containers WHERE deployment_id=? ORDER BY created_at DESC LIMIT 1`, strings.TrimSpace(deploymentID)).Scan(
		&item.ID, &item.DeploymentID, &item.DockerContainerID, &item.InternalPort, &item.HostPort, &item.Status, &createdAt, &updatedAt)
	if err != nil {
		return models.Container{}, fmt.Errorf("lookup container by deployment: %w", err)
	}
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, nil
}

func (s *Store) ListAllocatedHostPorts(ctx context.Context, excludeContainerID string) (map[int]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,host_port FROM containers WHERE status!='REMOVED' AND host_port>0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]struct{}{}
	excludeContainerID = strings.TrimSpace(excludeContainerID)
	for rows.Next() {
		var id string
		var port int
		if err := rows.Scan(&id, &port); err != nil {
			return nil, err
		}
		if id != excludeContainerID {
			out[port] = struct{}{}
		}
	}
	return out, rows.Err()
}

func (s *Store) UpdateContainerStatus(ctx context.Context, containerID, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE containers SET status=?,updated_at=? WHERE id=?`, strings.TrimSpace(status), time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(containerID))
	return err
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") && strings.Contains(message, "constraint")
}

func (s *Store) ListAllDomains(ctx context.Context) ([]models.Domain, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,application_id,environment_id,service_id,domain_name,kind,ssl_status,last_cert_message,cert_checked_at,created_at,updated_at FROM domains ORDER BY domain_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Domain{}
	for rows.Next() {
		var item models.Domain
		var createdAt, updatedAt string
		if err := rows.Scan(&item.ID, &item.ApplicationID, &item.EnvironmentID, &item.ServiceID, &item.DomainName, &item.Kind, &item.SSLStatus, &item.LastCertMessage, &item.CertCheckedAtRaw, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(createdAt)
		item.UpdatedAt = parseTime(updatedAt)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpdateDomainCertObservation(ctx context.Context, domainID, message string, checkedAt time.Time) error {
	checked := ""
	if !checkedAt.IsZero() {
		checked = checkedAt.UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE domains SET last_cert_message=?,cert_checked_at=? WHERE id=?`, strings.TrimSpace(message), checked, strings.TrimSpace(domainID))
	return err
}

func (s *Store) UpdateDomainSSLStatus(ctx context.Context, domainID, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE domains SET ssl_status=?,updated_at=? WHERE id=?`, strings.TrimSpace(status), time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(domainID))
	return err
}

func (s *Store) ListDomainRoutes(ctx context.Context) ([]models.DomainRoute, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id,d.application_id,d.environment_id,d.service_id,d.domain_name,d.kind,d.ssl_status,
		       d.last_cert_message,d.cert_checked_at,d.created_at,d.updated_at,c.host_port
		FROM domains d
		JOIN service_environments se ON se.service_id=d.service_id AND se.environment_id=d.environment_id
		LEFT JOIN containers c ON c.deployment_id=se.active_deployment_id AND c.status='RUNNING'
		ORDER BY d.domain_name`)
	if err != nil {
		return nil, fmt.Errorf("list domain routes: %w", err)
	}
	defer rows.Close()
	out := []models.DomainRoute{}
	for rows.Next() {
		var item models.DomainRoute
		var createdAt, updatedAt string
		var hostPort sql.NullInt64
		if err := rows.Scan(&item.ID, &item.ApplicationID, &item.EnvironmentID, &item.ServiceID, &item.DomainName, &item.Kind, &item.SSLStatus, &item.LastCertMessage, &item.CertCheckedAtRaw, &createdAt, &updatedAt, &hostPort); err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(createdAt)
		item.UpdatedAt = parseTime(updatedAt)
		if hostPort.Valid {
			item.HostPort = int(hostPort.Int64)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func parseTime(raw string) time.Time {
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return value
}

func newID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(value)
}
