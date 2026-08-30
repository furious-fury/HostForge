package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestControlPlaneSnapshotCRUD(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateControlPlaneSnapshot(ctx)
	if err != nil {
		t.Fatalf("CreateControlPlaneSnapshot: %v", err)
	}
	if created.Status != "running" {
		t.Fatalf("created status = %q, want running", created.Status)
	}
	if created.SnapshotPath != "" {
		t.Fatalf("created SnapshotPath = %q, want empty", created.SnapshotPath)
	}
	if !created.CompletedAt.IsZero() {
		t.Fatalf("created CompletedAt = %v, want zero", created.CompletedAt)
	}

	got, err := store.GetControlPlaneSnapshot(ctx, created.ID)
	if err != nil || got.ID != created.ID {
		t.Fatalf("GetControlPlaneSnapshot: got=%+v err=%v", got, err)
	}

	completed, err := store.CompleteControlPlaneSnapshot(ctx, created.ID, CompleteControlPlaneSnapshotInput{
		Status: "success", SnapshotPath: "/data/control-plane-snapshots/x.sqlite", RemoteKey: "control-plane/x.sqlite", SizeBytes: 4096,
	})
	if err != nil {
		t.Fatalf("CompleteControlPlaneSnapshot: %v", err)
	}
	if completed.Status != "success" || completed.SnapshotPath != "/data/control-plane-snapshots/x.sqlite" || completed.RemoteKey != "control-plane/x.sqlite" || completed.SizeBytes != 4096 {
		t.Fatalf("unexpected completed snapshot: %+v", completed)
	}
	if completed.CompletedAt.IsZero() {
		t.Fatal("completed CompletedAt is zero, want set")
	}

	listed, err := store.ListControlPlaneSnapshots(ctx, 0)
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("ListControlPlaneSnapshots: listed=%+v err=%v", listed, err)
	}

	latest, err := store.LatestControlPlaneSnapshot(ctx)
	if err != nil || latest.ID != created.ID {
		t.Fatalf("LatestControlPlaneSnapshot: latest=%+v err=%v", latest, err)
	}

	if err := store.DeleteControlPlaneSnapshotRecord(ctx, created.ID); err != nil {
		t.Fatalf("DeleteControlPlaneSnapshotRecord: %v", err)
	}
	if _, err := store.GetControlPlaneSnapshot(ctx, created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetControlPlaneSnapshot after delete: err=%v, want sql.ErrNoRows", err)
	}
}

func TestCompleteControlPlaneSnapshotRejectsInvalidStatus(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateControlPlaneSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteControlPlaneSnapshot(ctx, created.ID, CompleteControlPlaneSnapshotInput{Status: "running"}); err == nil {
		t.Fatal("expected error completing with a non-terminal status")
	}
}

func TestCompleteControlPlaneSnapshotUnknownIDReturnsNoRows(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.CompleteControlPlaneSnapshot(ctx, "does-not-exist", CompleteControlPlaneSnapshotInput{Status: "success"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestListExpiredControlPlaneSnapshotsRespectsRetention(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	old, err := store.CreateControlPlaneSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteControlPlaneSnapshot(ctx, old.ID, CompleteControlPlaneSnapshotInput{Status: "success", SnapshotPath: "/data/old.sqlite"}); err != nil {
		t.Fatal(err)
	}
	// Backdate created_at directly, matching database_backups_test.go's
	// idiom for exercising a retention cutoff without waiting real time.
	if _, err := store.db.ExecContext(ctx, `UPDATE control_plane_snapshots SET created_at=? WHERE id=?`,
		time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339), old.ID); err != nil {
		t.Fatal(err)
	}

	recent, err := store.CreateControlPlaneSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteControlPlaneSnapshot(ctx, recent.ID, CompleteControlPlaneSnapshotInput{Status: "success", SnapshotPath: "/data/recent.sqlite"}); err != nil {
		t.Fatal(err)
	}

	cutoff := time.Now().UTC().Add(-14 * 24 * time.Hour)
	expired, err := store.ListExpiredControlPlaneSnapshots(ctx, cutoff, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0].ID != old.ID {
		t.Fatalf("ListExpiredControlPlaneSnapshots = %+v, want exactly the backdated row", expired)
	}
}

func TestVacuumIntoWritesReadableSnapshot(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.CreateApplication(ctx, "Snapshot Target", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateControlPlaneSnapshot(ctx); err != nil {
		t.Fatal(err)
	}

	snapshotPath := filepath.Join(t.TempDir(), "snapshot.sqlite")
	if err := store.VacuumInto(ctx, snapshotPath); err != nil {
		t.Fatalf("VacuumInto: %v", err)
	}

	snapshotDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(snapshotPath)+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshotDB.Close()

	var apps, snapshots int
	if err := snapshotDB.QueryRowContext(ctx, `SELECT COUNT(1) FROM applications`).Scan(&apps); err != nil {
		t.Fatalf("read applications from snapshot: %v", err)
	}
	if err := snapshotDB.QueryRowContext(ctx, `SELECT COUNT(1) FROM control_plane_snapshots`).Scan(&snapshots); err != nil {
		t.Fatalf("read control_plane_snapshots from snapshot: %v", err)
	}
	if apps != 1 || snapshots != 1 {
		t.Fatalf("snapshot row counts = applications:%d control_plane_snapshots:%d, want 1 and 1", apps, snapshots)
	}
}

func TestVacuumIntoRejectsEmptyPath(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.VacuumInto(ctx, "  "); err == nil {
		t.Fatal("expected error for an empty/whitespace path")
	}
}
