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

type CreateServiceDeploymentInput struct {
	ServiceID     string
	EnvironmentID string
	CommitHash    string
	LogsPath      string
	ImageRef      string
	Worktree      string
	TriggerKind   string
	Actor         string
	RollbackOf    string
	Branch        string
}

func (s *Store) CreateServiceDeployment(ctx context.Context, in CreateServiceDeploymentInput) (models.Deployment, error) {
	in.ServiceID = strings.TrimSpace(in.ServiceID)
	in.EnvironmentID = strings.TrimSpace(in.EnvironmentID)
	if in.ServiceID == "" || in.EnvironmentID == "" {
		return models.Deployment{}, fmt.Errorf("service and environment are required")
	}
	var bindingCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM service_environments se
		JOIN services svc ON svc.id = se.service_id
		JOIN environments env ON env.id = se.environment_id AND env.application_id = svc.application_id
		WHERE se.service_id=? AND se.environment_id=?`, in.ServiceID, in.EnvironmentID).Scan(&bindingCount); err != nil {
		return models.Deployment{}, fmt.Errorf("validate service environment: %w", err)
	}
	if bindingCount == 0 {
		return models.Deployment{}, ErrEnvironmentNotFound
	}
	trigger := strings.TrimSpace(in.TriggerKind)
	if trigger == "" {
		trigger = "manual"
	}
	now := time.Now().UTC()
	item := models.Deployment{
		ID:            newID(),
		ServiceID:     in.ServiceID,
		EnvironmentID: in.EnvironmentID,
		Status:        models.DeploymentQueued,
		CommitHash:    strings.TrimSpace(in.CommitHash),
		LogsPath:      strings.TrimSpace(in.LogsPath),
		ImageRef:      strings.TrimSpace(in.ImageRef),
		Worktree:      strings.TrimSpace(in.Worktree),
		TriggerKind:   trigger,
		Actor:         strings.TrimSpace(in.Actor),
		RollbackOf:    strings.TrimSpace(in.RollbackOf),
		Branch:        strings.TrimSpace(in.Branch),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO deployments(
			id,service_id,environment_id,status,commit_hash,logs_path,image_ref,
			worktree,error_message,builder_kind,stack_kind,stack_label,trigger_kind,actor,
			cancelled_at,rollback_of,branch,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.ID, item.ServiceID, item.EnvironmentID, item.Status, item.CommitHash,
		item.LogsPath, item.ImageRef, item.Worktree, "", "", "", "", item.TriggerKind, item.Actor,
		"", item.RollbackOf, item.Branch, now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return models.Deployment{}, fmt.Errorf("insert service deployment: %w", err)
	}
	if err := s.RecordDeploymentEvent(ctx, item.ID, item.Status, ""); err != nil {
		return models.Deployment{}, err
	}
	return item, nil
}

func scanServiceDeployment(scanner interface{ Scan(...any) error }) (models.Deployment, error) {
	var item models.Deployment
	var created, updated string
	err := scanner.Scan(
		&item.ID, &item.ServiceID, &item.EnvironmentID, &item.Status,
		&item.CommitHash, &item.LogsPath, &item.ImageRef, &item.Worktree, &item.ErrorMessage,
		&item.BuilderKind, &item.StackKind, &item.StackLabel, &item.TriggerKind, &item.Actor,
		&item.CancelledAt, &item.RollbackOf, &item.Branch, &created, &updated,
	)
	if err != nil {
		return models.Deployment{}, err
	}
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

const serviceDeploymentColumns = `id,service_id,environment_id,status,commit_hash,logs_path,image_ref,worktree,error_message,builder_kind,stack_kind,stack_label,trigger_kind,actor,cancelled_at,rollback_of,branch,created_at,updated_at`

func (s *Store) GetServiceDeployment(ctx context.Context, id string) (models.Deployment, error) {
	item, err := scanServiceDeployment(s.db.QueryRowContext(ctx, `SELECT `+serviceDeploymentColumns+` FROM deployments WHERE id=?`, strings.TrimSpace(id)))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Deployment{}, sql.ErrNoRows
	}
	return item, err
}

func (s *Store) ListServiceDeployments(ctx context.Context, serviceID, environmentID string, limit int) ([]models.Deployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT ` + serviceDeploymentColumns + ` FROM deployments WHERE (?='' OR service_id=?) AND (?='' OR environment_id=?) ORDER BY created_at DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, strings.TrimSpace(serviceID), strings.TrimSpace(serviceID), strings.TrimSpace(environmentID), strings.TrimSpace(environmentID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Deployment, 0)
	for rows.Next() {
		item, err := scanServiceDeployment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetLatestSuccessfulServiceDeployment(ctx context.Context, serviceID, environmentID string) (models.Deployment, error) {
	return scanServiceDeployment(s.db.QueryRowContext(ctx, `SELECT `+serviceDeploymentColumns+` FROM deployments WHERE service_id=? AND environment_id=? AND status=? ORDER BY created_at DESC LIMIT 1`, strings.TrimSpace(serviceID), strings.TrimSpace(environmentID), models.DeploymentSuccess))
}

func (s *Store) ActivateServiceDeployment(ctx context.Context, serviceID, environmentID, deploymentID string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE service_environments SET active_deployment_id=?,desired_state='running',updated_at=? WHERE service_id=? AND environment_id=?`, strings.TrimSpace(deploymentID), time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(serviceID), strings.TrimSpace(environmentID))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrEnvironmentNotFound
	}
	return nil
}

func (s *Store) SetServiceDesiredState(ctx context.Context, serviceID, environmentID, state string) error {
	if state != "running" && state != "stopped" {
		return fmt.Errorf("invalid desired state")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE service_environments SET desired_state=?,updated_at=? WHERE service_id=? AND environment_id=?`, state, time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(serviceID), strings.TrimSpace(environmentID))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrEnvironmentNotFound
	}
	return nil
}

func (s *Store) CancelDeployment(ctx context.Context, id string) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `UPDATE deployments SET status=?,cancelled_at=?,updated_at=? WHERE id=? AND status IN (?,?)`, models.DeploymentCancelled, now, now, strings.TrimSpace(id), models.DeploymentQueued, models.DeploymentBuilding)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 1 {
		if err := s.RecordDeploymentEvent(ctx, id, models.DeploymentCancelled, "cancelled by operator"); err != nil {
			return false, err
		}
	}
	return n == 1, nil
}

type ServiceDeploymentFilter struct {
	ApplicationID string
	ServiceID     string
	EnvironmentID string
	Status        string
	Trigger       string
	Branch        string
	DateFrom      string
	DateTo        string
	Cursor        string
	Limit         int
}

func (s *Store) ListServiceDeploymentsFiltered(ctx context.Context, filter ServiceDeploymentFilter) ([]models.Deployment, string, error) {
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 100
	}
	query := `SELECT d.id,d.service_id,d.environment_id,d.status,d.commit_hash,d.logs_path,d.image_ref,d.worktree,d.error_message,d.builder_kind,d.stack_kind,d.stack_label,d.trigger_kind,d.actor,d.cancelled_at,d.rollback_of,d.branch,d.created_at,d.updated_at FROM deployments d
		LEFT JOIN services svc ON svc.id=d.service_id WHERE d.service_id<>''`
	args := make([]any, 0, 18)
	add := func(clause string, value any) {
		query += " AND " + clause
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.ApplicationID); value != "" {
		add("svc.application_id=?", value)
	}
	if value := strings.TrimSpace(filter.ServiceID); value != "" {
		add("d.service_id=?", value)
	}
	if value := strings.TrimSpace(filter.EnvironmentID); value != "" {
		add("d.environment_id=?", value)
	}
	if value := strings.ToUpper(strings.TrimSpace(filter.Status)); value != "" {
		add("d.status=?", value)
	}
	if value := strings.TrimSpace(filter.Trigger); value != "" {
		add("d.trigger_kind=?", value)
	}
	if value := strings.TrimSpace(filter.Branch); value != "" {
		add("d.branch=?", value)
	}
	if value := strings.TrimSpace(filter.DateFrom); value != "" {
		add("d.created_at>=?", value)
	}
	if value := strings.TrimSpace(filter.DateTo); value != "" {
		add("d.created_at<=?", value)
	}
	if value := strings.TrimSpace(filter.Cursor); value != "" {
		query += ` AND (
			d.created_at < (SELECT created_at FROM deployments WHERE id=?)
			OR (d.created_at = (SELECT created_at FROM deployments WHERE id=?) AND d.id < ?)
		)`
		args = append(args, value, value, value)
	}
	query += " ORDER BY d.created_at DESC,d.id DESC LIMIT ?"
	args = append(args, filter.Limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := make([]models.Deployment, 0, filter.Limit+1)
	for rows.Next() {
		item, scanErr := scanServiceDeployment(rows)
		if scanErr != nil {
			return nil, "", scanErr
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > filter.Limit {
		next = out[filter.Limit-1].ID
		out = out[:filter.Limit]
	}
	return out, next, nil
}
