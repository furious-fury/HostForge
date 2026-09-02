package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestDB opens a fresh, unmigrated SQLite database in a temp dir.
func openTestDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), name)
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", filepath.ToSlash(dbPath)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// embeddedMigrationNames returns every embedded migration filename, in
// numeric order — the same list ApplyMigrations itself computes.
func embeddedMigrationNames(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Slice(names, func(i, j int) bool {
		ni, _ := migrationNumber(names[i])
		nj, _ := migrationNumber(names[j])
		return ni < nj
	})
	return names
}

// seedAppliedThrough runs every embedded migration except those named in
// exclude, for real, against db — leaving the database in the state a real
// upgrade scenario would be in: schema and rows both genuinely reflect only
// the migrations that "already ran". This deliberately uses the real
// embedded migration SQL, not a fabricated stand-in schema.
func seedAppliedThrough(t *testing.T, ctx context.Context, db *sql.DB, exclude ...string) {
	t.Helper()
	excluded := make(map[string]bool, len(exclude))
	for _, name := range exclude {
		excluded[name] = true
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version TEXT PRIMARY KEY,
	applied_at TEXT NOT NULL
);`); err != nil {
		t.Fatalf("seed schema_migrations table: %v", err)
	}
	for _, name := range embeddedMigrationNames(t) {
		if excluded[name] {
			continue
		}
		body, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if err := applyMigration(ctx, db, name, string(body)); err != nil {
			t.Fatalf("seed migration %s: %v", name, err)
		}
	}
}

type migrationRow struct {
	version   string
	appliedAt string
}

func snapshotSchemaMigrations(t *testing.T, ctx context.Context, db *sql.DB) []migrationRow {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT version, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("snapshot schema_migrations: %v", err)
	}
	defer rows.Close()
	var out []migrationRow
	for rows.Next() {
		var r migrationRow
		if err := rows.Scan(&r.version, &r.appliedAt); err != nil {
			t.Fatalf("scan schema_migrations row: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema_migrations: %v", err)
	}
	return out
}

// The load-bearing case: a database that is merely behind the binary (the
// normal upgrade path) must not be rejected by the new downgrade check, and
// every migration still pending must actually run — not just get recorded.
func TestApplyMigrationsAppliesPendingMigrationsOnNormalUpgrade(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDB(t, "normal-upgrade.db")

	const (
		canaryMigration   = "0024_encryption_canary.sql"
		railpackMigration = "0025_deployment_railpack_artifacts.sql"
	)
	seedAppliedThrough(t, ctx, db, canaryMigration, railpackMigration)

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	want := len(embeddedMigrationNames(t))
	var got int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("schema_migrations has %d rows, want %d", got, want)
	}

	// The DDL effects of the two previously-pending migrations must have
	// actually run, not merely been marked applied.
	if _, err := db.ExecContext(ctx, `SELECT id FROM encryption_canary LIMIT 0`); err != nil {
		t.Errorf("encryption_canary table missing after upgrade: %v", err)
	}
	if _, err := db.ExecContext(ctx, `SELECT railpack_plan_json, railpack_info_json FROM deployments LIMIT 0`); err != nil {
		t.Errorf("deployments railpack columns missing after upgrade: %v", err)
	}
}

// A database last written by a newer binary (schema_migrations records a
// migration this binary doesn't embed) must be rejected before anything is
// touched — not partially migrated, not silently accepted.
func TestApplyMigrationsRejectsDowngrade(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDB(t, "downgrade.db")

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("initial ApplyMigrations: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, applied_at) VALUES(?, datetime('now'))`,
		"0099_future_fake.sql",
	); err != nil {
		t.Fatalf("seed future migration row: %v", err)
	}

	before := snapshotSchemaMigrations(t, ctx, db)

	err := ApplyMigrations(ctx, db)
	if err == nil {
		t.Fatal("ApplyMigrations: expected error for downgrade, got nil")
	}
	if !errors.Is(err, ErrSchemaNewerThanBinary) {
		t.Fatalf("ApplyMigrations error = %v, want it to wrap ErrSchemaNewerThanBinary", err)
	}

	after := snapshotSchemaMigrations(t, ctx, db)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("schema_migrations changed on a rejected downgrade:\nbefore=%+v\nafter=%+v", before, after)
	}
}

// A brand-new, empty database must migrate cleanly. This is the case that
// would have caught a naive downgrade check treating "no rows yet" as a
// downgrade (sql.ErrNoRows on the highest-applied lookup must mean "nothing
// applied", not "reject").
func TestApplyMigrationsOnFreshDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDB(t, "fresh.db")

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("ApplyMigrations on fresh database: %v", err)
	}

	want := len(embeddedMigrationNames(t))
	var got int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("schema_migrations has %d rows after fresh install, want %d", got, want)
	}
}
