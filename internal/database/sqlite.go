package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// OpenSQLite opens the SQLite database at dbPath, pings it, snapshots it if
// any migration is pending, then runs ApplyMigrations.
// The DSN uses WAL and a busy timeout to reduce "database is locked" under concurrent readers.
// MaxOpenConns(1) matches SQLite’s typical single-writer usage on the control plane.
//
// Every pragma must use the `_pragma=name(value)` form. The driver is
// modernc.org/sqlite, which recognises only that syntax; the `_busy_timeout=`
// and `_journal_mode=` query parameters are mattn/go-sqlite3 spellings and are
// silently discarded here.
func OpenSQLite(ctx context.Context, dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite is happiest with one connection for concurrent-safe writes on a single file.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	pending, err := PendingMigrations(ctx, db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if len(pending) > 0 {
		if err := snapshotBeforeMigrations(ctx, db, dbPath); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pre-migration snapshot: %w", err)
		}
	}

	if err := ApplyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// snapshotBeforeMigrations writes a VACUUM INTO snapshot of the database,
// named after dbPath, before any pending migration runs (ADR-0002 §17.3).
//
// This replaces the old backupBeforeApplicationModelMigration, which did a
// raw io.Copy of the whole file, once ever, keyed to a single fixed
// filename. VACUUM INTO produces a transactionally consistent snapshot
// under WAL without pausing writers, where an io.Copy of the raw db file
// can tear across the main file and the WAL. This runs once per boot that
// has at least one pending migration — i.e. roughly once per version
// upgrade, not once ever and not on every boot (a restart with nothing
// pending is the common case and does no extra I/O here).
//
// A snapshot failure blocks startup and ApplyMigrations is never reached —
// the same fail-closed behaviour the function this replaces already had: a
// database is never migrated unless a safety copy of its pre-migration
// state was written first.
//
// These snapshots are not tracked in any table and are never purged
// automatically — they accumulate at roughly one file per upgrade, a low
// rate; an operator reclaims space by deleting old
// "*.pre-migration-*.snapshot" files by hand. This is a different,
// lower-value problem than the retained, tracked snapshots taken by
// internal/services.StartControlPlaneSnapshotLoop.
func snapshotBeforeMigrations(ctx context.Context, db *sql.DB, dbPath string) error {
	snapshotPath := fmt.Sprintf("%s.pre-migration-%s.snapshot", dbPath, time.Now().UTC().Format("20060102T150405Z"))
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, snapshotPath); err != nil {
		return fmt.Errorf("vacuum into %s: %w", snapshotPath, err)
	}
	return nil
}
