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
	deploymentID = strings.TrimSpace(deploymentID)
	errorMessage = strings.TrimSpace(errorMessage)
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// CANCELLED is final. An operator's cancellation is written synchronously
	// by CancelDeployment while the deploy is still running, and the deploy
	// only learns it was cancelled at its next step boundary -- so without
	// this guard the work still in flight overwrites the cancellation with
	// whatever it went on to conclude. Observed on a real host: a cancelled
	// deploy finished its health check and rewrote the row as FAILED, leaving
	// cancelled_at set alongside an error message. Had the health check
	// passed it would have cut over and written SUCCESS instead, putting a
	// cancelled deploy into production.
	result, err := tx.ExecContext(ctx,
		`UPDATE deployments SET status=?,error_message=?,updated_at=? WHERE id=? AND status<>?`,
		status, errorMessage, now, deploymentID, models.DeploymentCancelled)
	if err != nil {
		return fmt.Errorf("update deployment status: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	if err := recordDeploymentStatusEventTx(ctx, tx, deploymentID, status, errorMessage, now); err != nil {
		return err
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

// UpdateDeploymentRailpackArtifacts persists the raw Railpack build plan and
// info for one deployment (ADR-0002 §15.6/§15.7). A separate method from
// UpdateDeploymentBuilder rather than an extension of it: a Dockerfile build
// has no plan/info to report, and this lets deploy.go skip the write
// entirely instead of passing two empty strings through every builder path.
// planJSON/infoJSON are stored byte-exact, not trimmed — trimming JSON
// content data is pointless and this keeps the stored value provably
// identical to what was captured.
func (s *Store) UpdateDeploymentRailpackArtifacts(ctx context.Context, deploymentID, planJSON, infoJSON string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET railpack_plan_json=?,railpack_info_json=?,updated_at=? WHERE id=?`, planJSON, infoJSON, time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(deploymentID))
	return err
}

// GetDeploymentRailpackArtifacts returns the raw Railpack plan/info for one
// deployment. Deliberately not part of scanServiceDeployment or any listing
// query: these columns can run tens of KB, deployment rows are never pruned,
// and the deployments list is polled by the UI every few seconds — loading
// this on every row of every poll would be pure waste for data almost no
// request needs. Callers must apply the same authorization as service
// configuration: a stored plan enumerates the build's environment variable
// names (never values — Prepare passes placeholders, not secrets), which is
// exposure parity with the already-plaintext deploy_install_cmd column, not
// a new class of data, but should not be handed out more freely than that.
func (s *Store) GetDeploymentRailpackArtifacts(ctx context.Context, deploymentID string) (planJSON, infoJSON string, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT railpack_plan_json,railpack_info_json FROM deployments WHERE id=?`, strings.TrimSpace(deploymentID)).Scan(&planJSON, &infoJSON)
	if err != nil {
		return "", "", fmt.Errorf("get deployment railpack artifacts: %w", err)
	}
	return planJSON, infoJSON, nil
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

// ListRetainedImageRefs returns the set of deploy image refs that must survive
// image garbage collection. An image is retained when any of these holds:
//
//   - a container that is not REMOVED references its deployment (the live
//     container, or an in-flight deploy's candidate -- the race a GC must never
//     lose, since it would delete an image a deploy is about to cut over to),
//   - its deployment is QUEUED or BUILDING (in flight, container maybe not yet
//     created),
//   - it is among the newest `retain` SUCCESS deployments for its service and
//     environment (the rollback/redeploy buffer).
//
// Rollback rebuilds from the source commit rather than reusing a stored image
// (see cmd/server handleDeploymentRollbackV2), so the buffer is a churn and
// race margin, not a correctness requirement -- `retain` can be small, and 0
// keeps only the in-use and in-flight images.
func (s *Store) ListRetainedImageRefs(ctx context.Context, retain int) (map[string]struct{}, error) {
	if retain < 0 {
		retain = 0
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.image_ref
		FROM deployments d
		JOIN containers c ON c.deployment_id=d.id
		WHERE c.status<>'REMOVED' AND d.image_ref<>''
		UNION
		SELECT image_ref FROM deployments
		WHERE status IN ('QUEUED','BUILDING') AND image_ref<>''
		UNION
		SELECT d.image_ref
		FROM deployments d
		WHERE d.status='SUCCESS' AND d.image_ref<>''
		  AND (
		    SELECT COUNT(*) FROM deployments d2
		    WHERE d2.service_id=d.service_id AND d2.environment_id=d.environment_id
		      AND d2.status='SUCCESS'
		      AND (d2.created_at>d.created_at OR (d2.created_at=d.created_at AND d2.id>d.id))
		  ) < ?`, retain)
	if err != nil {
		return nil, fmt.Errorf("list retained image refs: %w", err)
	}
	defer rows.Close()
	keep := map[string]struct{}{}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("scan retained image ref: %w", err)
		}
		keep[ref] = struct{}{}
	}
	return keep, rows.Err()
}

// SweepableContainer is a still-RUNNING container row whose deployment ended
// in a terminal state and is not the one currently serving its binding -- the
// signature of a container a crash left behind. The Docker id and the identity
// fields let the caller inspect the container and confirm ownership before it
// removes anything.
type SweepableContainer struct {
	ContainerRowID    string
	DockerContainerID string
	DeploymentID      string
	ServiceID         string
	EnvironmentID     string
}

// ListSweepableDeployContainers returns RUNNING application-container rows whose
// deployment is terminal (FAILED or CANCELLED) and is not the active deployment
// for its service and environment. That is exactly the residue an ungracefully
// killed deploy leaves: AttachContainer writes the row RUNNING right after the
// container starts (internal/services/deploy.go), so a process killed before
// cutover leaves the row RUNNING while recovery fails the deployment -- yet the
// container keeps running because a SIGKILL runs no cleanup.
//
// The status filter alone is nearly sufficient: active_deployment_id is only
// ever set to a deployment that reached cutover, which requires SUCCESS, so a
// terminal deployment is never the active one. The explicit inequality is kept
// as a guard on that invariant, cheap against the status index.
func (s *Store) ListSweepableDeployContainers(ctx context.Context) ([]SweepableContainer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id,c.docker_container_id,c.deployment_id,d.service_id,d.environment_id
		FROM containers c
		JOIN deployments d ON d.id=c.deployment_id
		LEFT JOIN service_environments se
		  ON se.service_id=d.service_id AND se.environment_id=d.environment_id
		WHERE c.status='RUNNING'
		  AND c.docker_container_id<>''
		  AND d.status IN ('FAILED','CANCELLED')
		  AND (se.active_deployment_id IS NULL OR se.active_deployment_id<>c.deployment_id)
		ORDER BY c.created_at`)
	if err != nil {
		return nil, fmt.Errorf("list sweepable deploy containers: %w", err)
	}
	defer rows.Close()
	out := []SweepableContainer{}
	for rows.Next() {
		var item SweepableContainer
		if err := rows.Scan(&item.ContainerRowID, &item.DockerContainerID, &item.DeploymentID, &item.ServiceID, &item.EnvironmentID); err != nil {
			return nil, fmt.Errorf("scan sweepable deploy container: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
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
