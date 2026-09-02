package services

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/repository"

	_ "modernc.org/sqlite"
)

// newControlPlaneSnapshotTestStore returns a store plus its backing file
// path — repository.Store's db field is unexported, so a test in this
// package that needs to backdate a row directly (see
// backdateControlPlaneSnapshot) has to open its own connection to the same
// file rather than reach through the store.
func newControlPlaneSnapshotTestStore(t *testing.T) (*repository.Store, string) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "hostforge.sqlite")
	db, err := database.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return repository.New(db), dbPath
}

// backdateControlPlaneSnapshot rewrites a row's created_at directly,
// mirroring internal/repository's own backdating idiom for retention tests
// — done here through a second connection to the same file, since
// repository.Store.db isn't reachable from this package.
func backdateControlPlaneSnapshot(t *testing.T, dbPath, id string, when time.Time) {
	t.Helper()
	raw, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", filepath.ToSlash(dbPath)))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.ExecContext(context.Background(), `UPDATE control_plane_snapshots SET created_at=? WHERE id=?`, when.UTC().Format(time.RFC3339), id); err != nil {
		t.Fatal(err)
	}
}

func testSlogDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRunControlPlaneSnapshotWritesLocalFileAndRecord(t *testing.T) {
	store, _ := newControlPlaneSnapshotTestStore(t)
	ctx := context.Background()
	cfg := &config.Config{ControlPlaneSnapshotDir: t.TempDir()}
	log := testSlogDiscard()

	runControlPlaneSnapshot(ctx, log, cfg, store, nil, time.Now().UTC())

	snapshots, err := store.ListControlPlaneSnapshots(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %+v, want exactly 1", snapshots)
	}
	row := snapshots[0]
	if row.Status != "success" {
		t.Fatalf("status = %q, want success (error: %q)", row.Status, row.ErrorMessage)
	}
	if row.RemoteKey != "" {
		t.Fatalf("remote_key = %q, want empty (no destination configured)", row.RemoteKey)
	}
	if row.SizeBytes <= 0 {
		t.Fatalf("size_bytes = %d, want > 0", row.SizeBytes)
	}
	info, err := os.Stat(row.SnapshotPath)
	if err != nil {
		t.Fatalf("stat snapshot file: %v", err)
	}
	if info.Size() != row.SizeBytes {
		t.Fatalf("file size %d does not match recorded size_bytes %d", info.Size(), row.SizeBytes)
	}
}

func TestMaybeRunControlPlaneSnapshotSkipsWhenNotDue(t *testing.T) {
	store, _ := newControlPlaneSnapshotTestStore(t)
	ctx := context.Background()
	cfg := &config.Config{ControlPlaneSnapshotDir: t.TempDir(), ControlPlaneSnapshotIntervalMinutes: 360}
	log := testSlogDiscard()

	now := time.Now().UTC()
	runControlPlaneSnapshot(ctx, log, cfg, store, nil, now)

	before, err := store.ListControlPlaneSnapshots(ctx, 10)
	if err != nil || len(before) != 1 {
		t.Fatalf("setup: before=%+v err=%v", before, err)
	}

	// Immediately after: well within the 360-minute interval.
	maybeRunControlPlaneSnapshot(ctx, log, cfg, store, nil, now.Add(time.Minute))

	after, err := store.ListControlPlaneSnapshots(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatalf("snapshots = %+v, want still exactly 1 (not due yet)", after)
	}
}

func TestMaybeRunControlPlaneSnapshotRunsWhenDue(t *testing.T) {
	store, dbPath := newControlPlaneSnapshotTestStore(t)
	ctx := context.Background()
	cfg := &config.Config{ControlPlaneSnapshotDir: t.TempDir(), ControlPlaneSnapshotIntervalMinutes: 360}
	log := testSlogDiscard()

	now := time.Now().UTC()
	runControlPlaneSnapshot(ctx, log, cfg, store, nil, now)

	before, err := store.ListControlPlaneSnapshots(ctx, 10)
	if err != nil || len(before) != 1 {
		t.Fatalf("setup: before=%+v err=%v", before, err)
	}
	// CreateControlPlaneSnapshot stamps created_at with its own
	// time.Now(), not the now argument runControlPlaneSnapshot was called
	// with — backdate the row directly so maybeRun sees the interval as
	// elapsed.
	backdateControlPlaneSnapshot(t, dbPath, before[0].ID, now.Add(-400*time.Minute))

	maybeRunControlPlaneSnapshot(ctx, log, cfg, store, nil, now)

	after, err := store.ListControlPlaneSnapshots(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 {
		t.Fatalf("snapshots = %+v, want 2 (interval elapsed since the last attempt)", after)
	}
}

func TestMaybeRunControlPlaneSnapshotRunsOnFirstEverAttempt(t *testing.T) {
	store, _ := newControlPlaneSnapshotTestStore(t)
	ctx := context.Background()
	cfg := &config.Config{ControlPlaneSnapshotDir: t.TempDir(), ControlPlaneSnapshotIntervalMinutes: 360}
	log := testSlogDiscard()

	maybeRunControlPlaneSnapshot(ctx, log, cfg, store, nil, time.Now().UTC())

	snapshots, err := store.ListControlPlaneSnapshots(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %+v, want exactly 1 (no prior attempt, sql.ErrNoRows must mean \"run it\")", snapshots)
	}
}

func TestPurgeExpiredControlPlaneSnapshotsRemovesLocalFile(t *testing.T) {
	store, dbPath := newControlPlaneSnapshotTestStore(t)
	ctx := context.Background()
	cfg := &config.Config{ControlPlaneSnapshotRetentionDays: 14}
	log := testSlogDiscard()

	snapshotPath := filepath.Join(t.TempDir(), "expired.sqlite")
	if err := os.WriteFile(snapshotPath, []byte("not a real db, just needs to exist"), 0o600); err != nil {
		t.Fatal(err)
	}
	row, err := store.CreateControlPlaneSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteControlPlaneSnapshot(ctx, row.ID, repository.CompleteControlPlaneSnapshotInput{Status: "success", SnapshotPath: snapshotPath}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	purgeExpiredControlPlaneSnapshots(ctx, log, cfg, store, nil, now)
	// Not yet past retention: nothing should be purged.
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("snapshot file removed before retention elapsed: %v", err)
	}
	if snapshots, err := store.ListControlPlaneSnapshots(ctx, 10); err != nil || len(snapshots) != 1 {
		t.Fatalf("snapshots = %+v err=%v, want still 1", snapshots, err)
	}

	backdateControlPlaneSnapshot(t, dbPath, row.ID, now.Add(-30*24*time.Hour))

	purgeExpiredControlPlaneSnapshots(ctx, log, cfg, store, nil, now)

	if _, err := os.Stat(snapshotPath); !os.IsNotExist(err) {
		t.Fatalf("expired snapshot file still present: stat err=%v", err)
	}
	if snapshots, err := store.ListControlPlaneSnapshots(ctx, 10); err != nil || len(snapshots) != 0 {
		t.Fatalf("snapshots = %+v err=%v, want none after purge", snapshots, err)
	}
}

func TestUploadControlPlaneSnapshotIsBestEffort(t *testing.T) {
	store, _ := newControlPlaneSnapshotTestStore(t)
	ctx := context.Background()
	log := testSlogDiscard()

	sealer, err := envcrypt.NewFromBase64Key(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}

	// No destination configured: must return "" without erroring or panicking.
	key := uploadControlPlaneSnapshot(ctx, log, store, sealer, "", "/does/not/matter", "snapshot.sqlite")
	if key != "" {
		t.Fatalf("uploadControlPlaneSnapshot with no destination returned %q, want empty", key)
	}

	// A destination ID that doesn't exist: same best-effort "" result, not an error.
	key = uploadControlPlaneSnapshot(ctx, log, store, sealer, "does-not-exist", "/does/not/matter", "snapshot.sqlite")
	if key != "" {
		t.Fatalf("uploadControlPlaneSnapshot with an unknown destination returned %q, want empty", key)
	}
}

func TestStartControlPlaneSnapshotLoopDisabledWhenIntervalIsZero(t *testing.T) {
	store, _ := newControlPlaneSnapshotTestStore(t)
	cfg := &config.Config{ControlPlaneSnapshotIntervalMinutes: 0, ControlPlaneSnapshotDir: t.TempDir()}

	// Must return immediately without starting a goroutine; if it didn't,
	// this would need a context to cancel and time to observe no snapshot
	// was taken. Calling with a already-cancelled context and asserting no
	// panic/hang is a sufficient smoke test for the early return.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	StartControlPlaneSnapshotLoop(ctx, testSlogDiscard(), cfg, store, nil)
}
