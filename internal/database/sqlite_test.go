package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

// The DSN in OpenSQLite must be written in the syntax the driver actually
// parses. modernc.org/sqlite reads `_pragma=name(value)`; the `_busy_timeout=`
// and `_journal_mode=` forms belong to mattn/go-sqlite3 and are ignored here,
// which silently leaves the database in its default rollback-journal mode.
func TestOpenSQLiteAppliesPragmas(t *testing.T) {
	db, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "pragma.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, tc := range []struct {
		pragma string
		want   string
	}{
		{"journal_mode", "wal"},
		{"busy_timeout", "5000"},
		{"foreign_keys", "1"},
		{"synchronous", "1"}, // NORMAL, the standard companion to WAL
	} {
		var got string
		if err := db.QueryRowContext(context.Background(), "PRAGMA "+tc.pragma).Scan(&got); err != nil {
			t.Fatalf("read PRAGMA %s: %v", tc.pragma, err)
		}
		if got != tc.want {
			t.Errorf("PRAGMA %s = %q, want %q", tc.pragma, got, tc.want)
		}
	}
}

// The whole pre-migration snapshot design leans on VACUUM INTO working with
// a bound parameter (not just a string literal) against modernc.org/sqlite
// — this proves it directly, and that the resulting file is a valid,
// readable SQLite database with the same content as the source.
func TestVacuumIntoSyntaxIsSupportedByDriver(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "source.db")
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_busy_timeout=5000", filepath.ToSlash(dbPath)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, `CREATE TABLE widgets(id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO widgets(name) VALUES('sprocket'),('cog')`); err != nil {
		t.Fatal(err)
	}

	snapshotPath := filepath.Join(dir, "snapshot.db")
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, snapshotPath); err != nil {
		t.Fatalf("VACUUM INTO with a bound parameter: %v", err)
	}

	snapshotDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_busy_timeout=5000", filepath.ToSlash(snapshotPath)))
	if err != nil {
		t.Fatal(err)
	}
	defer snapshotDB.Close()

	var count int
	if err := snapshotDB.QueryRowContext(ctx, `SELECT COUNT(1) FROM widgets`).Scan(&count); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if count != 2 {
		t.Fatalf("snapshot has %d widgets rows, want 2", count)
	}
}

// OpenSQLite must snapshot a database with pending migrations exactly once
// per boot, and must not snapshot again on a subsequent boot once nothing
// is pending — this is the regression risk the whole gate exists to guard.
func TestOpenSQLitePreMigrationSnapshotOnlyWhenPending(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "gated.db")

	db1, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("first OpenSQLite: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	snapshots, err := filepath.Glob(dbPath + ".pre-migration-*.snapshot")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("after first (fresh) open, snapshots = %v, want exactly 1", snapshots)
	}

	db2, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("second OpenSQLite: %v", err)
	}
	if err := db2.Close(); err != nil {
		t.Fatal(err)
	}

	snapshots, err = filepath.Glob(dbPath + ".pre-migration-*.snapshot")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("after second (fully-migrated) open, snapshots = %v, want still exactly 1 (no snapshot on a fully-migrated boot)", snapshots)
	}
}
