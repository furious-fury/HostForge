package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/models"
)

const (
	observabilityMaxRows   = 5000
	observabilityTrimBatch = 1000
)

// DeployStepRow is a row returned for API/UI.
type DeployStepRow struct {
	ID              int64  `json:"id"`
	DeploymentID    string `json:"deployment_id"`
	ServiceID       string `json:"service_id"`
	EnvironmentID   string `json:"environment_id"`
	RequestID       string `json:"request_id"`
	Step            string `json:"step"`
	Status          string `json:"status"`
	DurationMS      int64  `json:"duration_ms"`
	ErrorCode       string `json:"error_code"`
	StartedAt       string `json:"started_at"`
	EndedAt         string `json:"ended_at"`
	ServiceName     string `json:"service_name,omitempty"`
	EnvironmentName string `json:"environment_name,omitempty"`
}

// HTTPRequestRow is a row returned for API/UI.
type HTTPRequestRow struct {
	ID            int64  `json:"id"`
	RequestID     string `json:"request_id"`
	ApplicationID string `json:"application_id,omitempty"`
	ServiceID     string `json:"service_id,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	Status        int    `json:"status"`
	DurationMS    int64  `json:"duration_ms"`
	StartedAt     string `json:"started_at"`
}

type HTTPRequestFilter struct {
	ApplicationID string
	ServiceID     string
	EnvironmentID string
	Method        string
	StatusClass   string
	DateFrom      string
	DateTo        string
	Cursor        int64
	Limit         int
}

type DeployStepFilter struct {
	ApplicationID string
	ServiceID     string
	EnvironmentID string
	Status        string
	DateFrom      string
	DateTo        string
	Cursor        int64
	Limit         int
}

// ObservabilitySummary aggregates the last windowHours of data.
type ObservabilitySummary struct {
	WindowHours int `json:"window_hours"`

	HTTPRequestCount int64 `json:"http_request_count"`
	HTTPErrorCount   int64 `json:"http_error_count"`
	HTTPDurationP50  int64 `json:"http_duration_p50_ms"`
	HTTPDurationP95  int64 `json:"http_duration_p95_ms"`

	DeployCount       int64 `json:"deploy_count"`
	DeployFailedCount int64 `json:"deploy_failed_count"`
	DeployDurationP50 int64 `json:"deploy_duration_p50_ms"`
	DeployDurationP95 int64 `json:"deploy_duration_p95_ms"`
}

func formatObsTime(t time.Time) string {
	if t.IsZero() {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// InsertDeployStep appends a deploy step span and trims old rows if over cap.
func (s *Store) InsertDeployStep(ctx context.Context, in models.DeployStepRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO deploy_steps (deployment_id, service_id, environment_id, request_id, step, status, duration_ms, error_code, started_at, ended_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(in.DeploymentID),
		strings.TrimSpace(in.ServiceID),
		strings.TrimSpace(in.EnvironmentID),
		strings.TrimSpace(in.RequestID),
		in.Step,
		in.Status,
		in.DurationMS,
		strings.TrimSpace(in.ErrorCode),
		formatObsTime(in.StartedAt),
		formatObsTime(in.EndedAt),
	)
	if err != nil {
		return fmt.Errorf("insert deploy_step: %w", err)
	}
	return s.trimDeployStepsIfNeeded(ctx)
}

// InsertHTTPRequest records one HTTP request sample and trims if over cap.
func (s *Store) InsertHTTPRequest(ctx context.Context, in models.HTTPRequestRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO http_requests (request_id, application_id, service_id, environment_id, method, path, status, duration_ms, started_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(in.RequestID),
		strings.TrimSpace(in.ApplicationID),
		strings.TrimSpace(in.ServiceID),
		strings.TrimSpace(in.EnvironmentID),
		strings.TrimSpace(in.Method),
		strings.TrimSpace(in.Path),
		in.Status,
		in.DurationMS,
		formatObsTime(in.StartedAt),
	)
	if err != nil {
		return fmt.Errorf("insert http_request: %w", err)
	}
	return s.trimHTTPRequestsIfNeeded(ctx)
}

func (s *Store) trimDeployStepsIfNeeded(ctx context.Context) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM deploy_steps`).Scan(&n); err != nil {
		return err
	}
	if n <= observabilityMaxRows {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM deploy_steps WHERE id IN (
  SELECT id FROM deploy_steps ORDER BY datetime(ended_at) ASC LIMIT ?
)`, observabilityTrimBatch)
	return err
}

func (s *Store) trimHTTPRequestsIfNeeded(ctx context.Context) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM http_requests`).Scan(&n); err != nil {
		return err
	}
	if n <= observabilityMaxRows {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM http_requests WHERE id IN (
  SELECT id FROM http_requests ORDER BY datetime(started_at) ASC LIMIT ?
)`, observabilityTrimBatch)
	return err
}

// ListDeployStepsByDeployment returns steps for one deployment, newest first.
func (s *Store) ListDeployStepsByDeployment(ctx context.Context, deploymentID string, limit int) ([]DeployStepRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, deployment_id, service_id, environment_id, request_id, step, status, duration_ms, error_code, started_at, ended_at
FROM deploy_steps WHERE deployment_id = ? ORDER BY datetime(ended_at) DESC, id DESC LIMIT ?`,
		strings.TrimSpace(deploymentID), limit)
	if err != nil {
		return nil, fmt.Errorf("list deploy_steps by deployment: %w", err)
	}
	defer rows.Close()
	return scanDeployStepRows(rows)
}

// ListRecentDeploySteps returns recent steps across deployments (for observability page).
func (s *Store) ListRecentDeploySteps(ctx context.Context, limit int) ([]DeployStepRow, error) {
	rows, _, err := s.ListDeployStepsFiltered(ctx, DeployStepFilter{Limit: limit})
	return rows, err
}

func (s *Store) ListDeployStepsFiltered(ctx context.Context, filter DeployStepFilter) ([]DeployStepRow, int64, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query := `
SELECT ds.id,ds.deployment_id,ds.service_id,ds.environment_id,ds.request_id,ds.step,ds.status,ds.duration_ms,ds.error_code,ds.started_at,ds.ended_at,
       COALESCE(svc.name,''),COALESCE(env.name,'')
FROM deploy_steps ds
LEFT JOIN services svc ON svc.id=ds.service_id
LEFT JOIN environments env ON env.id=ds.environment_id
WHERE 1=1`
	args := make([]any, 0, 9)
	add := func(clause string, value any) {
		query += " AND " + clause
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.ApplicationID); value != "" {
		add("svc.application_id=?", value)
	}
	if value := strings.TrimSpace(filter.ServiceID); value != "" {
		add("ds.service_id=?", value)
	}
	if value := strings.TrimSpace(filter.EnvironmentID); value != "" {
		add("ds.environment_id=?", value)
	}
	if value := strings.TrimSpace(filter.Status); value != "" {
		add("ds.status=?", value)
	}
	if value := strings.TrimSpace(filter.DateFrom); value != "" {
		add("ds.ended_at>=?", value)
	}
	if value := strings.TrimSpace(filter.DateTo); value != "" {
		add("ds.ended_at<=?", value)
	}
	if filter.Cursor > 0 {
		add("ds.id<?", filter.Cursor)
	}
	query += " ORDER BY ds.id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list recent deploy_steps: %w", err)
	}
	defer rows.Close()
	out, err := scanDeployStepRowsWithContext(rows)
	if err != nil {
		return nil, 0, err
	}
	next := int64(0)
	if len(out) > limit {
		next = out[limit-1].ID
		out = out[:limit]
	}
	return out, next, nil
}

func scanDeployStepRows(rows *sql.Rows) ([]DeployStepRow, error) {
	var out []DeployStepRow
	for rows.Next() {
		var r DeployStepRow
		var dur sql.NullInt64
		if err := rows.Scan(&r.ID, &r.DeploymentID, &r.ServiceID, &r.EnvironmentID, &r.RequestID, &r.Step, &r.Status, &dur, &r.ErrorCode, &r.StartedAt, &r.EndedAt); err != nil {
			return nil, err
		}
		if dur.Valid {
			r.DurationMS = dur.Int64
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanDeployStepRowsWithContext(rows *sql.Rows) ([]DeployStepRow, error) {
	var out []DeployStepRow
	for rows.Next() {
		var r DeployStepRow
		var dur sql.NullInt64
		if err := rows.Scan(&r.ID, &r.DeploymentID, &r.ServiceID, &r.EnvironmentID, &r.RequestID, &r.Step, &r.Status, &dur, &r.ErrorCode, &r.StartedAt, &r.EndedAt, &r.ServiceName, &r.EnvironmentName); err != nil {
			return nil, err
		}
		if dur.Valid {
			r.DurationMS = dur.Int64
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRecentHTTPRequests returns recent HTTP samples, newest first.
func (s *Store) ListRecentHTTPRequests(ctx context.Context, limit int) ([]HTTPRequestRow, error) {
	rows, _, err := s.ListHTTPRequestsFiltered(ctx, HTTPRequestFilter{Limit: limit})
	return rows, err
}

func (s *Store) ListHTTPRequestsFiltered(ctx context.Context, filter HTTPRequestFilter) ([]HTTPRequestRow, int64, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
SELECT id, request_id, application_id, service_id, environment_id, method, path, status, duration_ms, started_at
FROM http_requests WHERE 1=1`
	args := make([]any, 0, 10)
	add := func(clause string, value any) {
		query += " AND " + clause
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.ApplicationID); value != "" {
		add("application_id=?", value)
	}
	if value := strings.TrimSpace(filter.ServiceID); value != "" {
		add("service_id=?", value)
	}
	if value := strings.TrimSpace(filter.EnvironmentID); value != "" {
		add("environment_id=?", value)
	}
	if value := strings.ToUpper(strings.TrimSpace(filter.Method)); value != "" {
		add("method=?", value)
	}
	switch strings.TrimSpace(filter.StatusClass) {
	case "success":
		query += " AND status>=200 AND status<400"
	case "client_error":
		query += " AND status>=400 AND status<500"
	case "server_error":
		query += " AND status>=500"
	}
	if value := strings.TrimSpace(filter.DateFrom); value != "" {
		add("started_at>=?", value)
	}
	if value := strings.TrimSpace(filter.DateTo); value != "" {
		add("started_at<=?", value)
	}
	if filter.Cursor > 0 {
		add("id<?", filter.Cursor)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list http_requests: %w", err)
	}
	defer rows.Close()
	out := make([]HTTPRequestRow, 0, limit+1)
	for rows.Next() {
		var r HTTPRequestRow
		if err := rows.Scan(&r.ID, &r.RequestID, &r.ApplicationID, &r.ServiceID, &r.EnvironmentID, &r.Method, &r.Path, &r.Status, &r.DurationMS, &r.StartedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	next := int64(0)
	if len(out) > limit {
		next = out[limit-1].ID
		out = out[:limit]
	}
	return out, next, nil
}

// SummarizeObservability aggregates metrics for the last windowHours.
func (s *Store) SummarizeObservability(ctx context.Context, windowHours int) (ObservabilitySummary, error) {
	if windowHours <= 0 {
		windowHours = 24
	}
	out := ObservabilitySummary{WindowHours: windowHours}
	cut := fmt.Sprintf("-%d hours", windowHours)

	// HTTP requests in window
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1) FROM http_requests WHERE datetime(started_at) >= datetime('now', ?)`, cut).Scan(&out.HTTPRequestCount); err != nil {
		return out, fmt.Errorf("http count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1) FROM http_requests WHERE datetime(started_at) >= datetime('now', ?) AND status >= 400`, cut).Scan(&out.HTTPErrorCount); err != nil {
		return out, fmt.Errorf("http errors: %w", err)
	}
	durs, err := s.queryInt64Slice(ctx, `
SELECT duration_ms FROM http_requests WHERE datetime(started_at) >= datetime('now', ?) AND duration_ms >= 0`, cut)
	if err != nil {
		return out, err
	}
	out.HTTPDurationP50, out.HTTPDurationP95 = percentileP50P95(durs)

	// Deployments created in window
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1) FROM deployments WHERE datetime(created_at) >= datetime('now', ?)`, cut).Scan(&out.DeployCount); err != nil {
		return out, fmt.Errorf("deploy count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1) FROM deployments WHERE datetime(created_at) >= datetime('now', ?) AND status = 'FAILED'`, cut).Scan(&out.DeployFailedCount); err != nil {
		return out, fmt.Errorf("deploy failed: %w", err)
	}

	deployTotals, err := s.queryInt64Slice(ctx, `
SELECT duration_ms FROM deploy_steps
WHERE step = 'deploy_total' AND datetime(ended_at) >= datetime('now', ?) AND duration_ms >= 0`, cut)
	if err != nil {
		return out, err
	}
	out.DeployDurationP50, out.DeployDurationP95 = percentileP50P95(deployTotals)

	return out, nil
}

func (s *Store) queryInt64Slice(ctx context.Context, query string, args ...any) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var vals []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
	return vals, rows.Err()
}

func percentileP50P95(vals []int64) (p50, p95 int64) {
	if len(vals) == 0 {
		return 0, 0
	}
	cp := append([]int64(nil), vals...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	p50 = cp[len(cp)/2]
	idx95 := int(float64(len(cp)-1) * 0.95)
	if idx95 < 0 {
		idx95 = 0
	}
	p95 = cp[idx95]
	return p50, p95
}
