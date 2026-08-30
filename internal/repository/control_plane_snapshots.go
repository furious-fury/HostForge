package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ControlPlaneSnapshot tracks one VACUUM INTO attempt taken by the scheduled
// control-plane snapshot loop (ADR-0002 §17.2). Distinct from the
// untracked, unscheduled pre-migration snapshot in
// internal/database.OpenSQLite, which never touches this table.
type ControlPlaneSnapshot struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	SnapshotPath string    `json:"snapshot_path,omitempty"`
	RemoteKey    string    `json:"remote_key,omitempty"`
	SizeBytes    int64     `json:"size_bytes"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
}

const controlPlaneSnapshotColumns = `id,status,snapshot_path,remote_key,size_bytes,error_message,created_at,completed_at`

func scanControlPlaneSnapshot(scanner interface{ Scan(...any) error }) (ControlPlaneSnapshot, error) {
	var item ControlPlaneSnapshot
	var created, completed string
	err := scanner.Scan(&item.ID, &item.Status, &item.SnapshotPath, &item.RemoteKey, &item.SizeBytes, &item.ErrorMessage, &created, &completed)
	item.CreatedAt, item.CompletedAt = parseTime(created), parseTime(completed)
	return item, err
}

// CreateControlPlaneSnapshot inserts a snapshot attempt row as 'running'.
// There is no queued/pending state: the caller performs the VACUUM INTO
// synchronously right after this call — no operations queue exists yet for
// scheduled background work (that's Phase 1 of the ADR, out of scope here).
func (s *Store) CreateControlPlaneSnapshot(ctx context.Context) (ControlPlaneSnapshot, error) {
	now, id := time.Now().UTC(), newID()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO control_plane_snapshots(id,status,created_at) VALUES(?,'running',?)`,
		id, now.Format(time.RFC3339)); err != nil {
		return ControlPlaneSnapshot{}, err
	}
	return s.GetControlPlaneSnapshot(ctx, id)
}

func (s *Store) GetControlPlaneSnapshot(ctx context.Context, id string) (ControlPlaneSnapshot, error) {
	return scanControlPlaneSnapshot(s.db.QueryRowContext(ctx, `SELECT `+controlPlaneSnapshotColumns+` FROM control_plane_snapshots WHERE id=?`, strings.TrimSpace(id)))
}

type CompleteControlPlaneSnapshotInput struct {
	Status, SnapshotPath, RemoteKey, ErrorMessage string
	SizeBytes                                     int64
}

func (s *Store) CompleteControlPlaneSnapshot(ctx context.Context, id string, in CompleteControlPlaneSnapshotInput) (ControlPlaneSnapshot, error) {
	if in.Status != "success" && in.Status != "failed" {
		return ControlPlaneSnapshot{}, fmt.Errorf("invalid control-plane snapshot completion status")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `
		UPDATE control_plane_snapshots
		SET status=?,snapshot_path=?,remote_key=?,size_bytes=?,error_message=?,completed_at=?
		WHERE id=?`, in.Status, strings.TrimSpace(in.SnapshotPath), strings.TrimSpace(in.RemoteKey), in.SizeBytes, strings.TrimSpace(in.ErrorMessage), now, strings.TrimSpace(id))
	if err != nil {
		return ControlPlaneSnapshot{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ControlPlaneSnapshot{}, sql.ErrNoRows
	}
	return s.GetControlPlaneSnapshot(ctx, id)
}

// LatestControlPlaneSnapshot returns the most recent snapshot attempt (any
// status), used by the scheduler to decide whether the configured interval
// has elapsed since the last attempt.
func (s *Store) LatestControlPlaneSnapshot(ctx context.Context) (ControlPlaneSnapshot, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM control_plane_snapshots ORDER BY created_at DESC,id DESC LIMIT 1`).Scan(&id)
	if err != nil {
		return ControlPlaneSnapshot{}, err
	}
	return s.GetControlPlaneSnapshot(ctx, id)
}

func (s *Store) ListControlPlaneSnapshots(ctx context.Context, limit int) ([]ControlPlaneSnapshot, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+controlPlaneSnapshotColumns+` FROM control_plane_snapshots ORDER BY created_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ControlPlaneSnapshot{}
	for rows.Next() {
		item, err := scanControlPlaneSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListExpiredControlPlaneSnapshots returns successful snapshots created at
// or before cutoff, for the retention purge.
func (s *Store) ListExpiredControlPlaneSnapshots(ctx context.Context, cutoff time.Time, limit int) ([]ControlPlaneSnapshot, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+controlPlaneSnapshotColumns+` FROM control_plane_snapshots WHERE status='success' AND created_at<=? ORDER BY created_at,id LIMIT ?`, cutoff.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ControlPlaneSnapshot{}
	for rows.Next() {
		item, err := scanControlPlaneSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) DeleteControlPlaneSnapshotRecord(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM control_plane_snapshots WHERE id=?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// VacuumInto issues VACUUM INTO to write a fully consistent, compacted
// snapshot of the entire database to path. Safe alongside concurrent
// readers/writers under WAL — VACUUM INTO does not require exclusive
// access. It still executes serially through this Store's single write
// connection (MaxOpenConns(1) — see internal/database/sqlite.go), so other
// writes queue behind it for its duration; keep the calling interval
// infrequent (the scheduled loop's default is 6h) rather than tight enough
// for that to matter.
func (s *Store) VacuumInto(ctx context.Context, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("vacuum into: path must not be empty")
	}
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path)
	return err
}
