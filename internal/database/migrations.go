package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// ErrSchemaNewerThanBinary is returned by ApplyMigrations when
// schema_migrations already records a migration numbered higher than any
// migration this binary embeds — i.e. the database was last written by a
// newer binary. Applying migrations in that state would skip nothing (there
// is nothing pending), but running application code against a schema newer
// than the binary understands is unsafe. The fix is to run a binary at
// least as new as whatever last wrote this database, never an older one.
var ErrSchemaNewerThanBinary = errors.New("database schema is newer than this binary; run a binary at least as new as whatever last wrote this database, not an older one")

// ApplyMigrations ensures schema_migrations exists, then runs each embedded migrations/*.sql
// file once, in numeric filename order. Filenames act as version keys (e.g. 0001_initial.sql);
// the leading digits, not the raw string, order them — a lexical sort would misorder once
// migration counts reach different digit widths (e.g. "10000_x.sql" before "9999_x.sql").
// Each migration runs in a single transaction: DDL + insert into schema_migrations.
//
// Before applying anything, the database's highest already-applied migration is compared
// against the highest migration this binary knows about. If the database is ahead, this
// returns ErrSchemaNewerThanBinary instead of touching the database — a downgrade (running
// an older binary against a newer database) is rejected outright rather than silently
// applying nothing and leaving the mismatch for application code to trip over later.
func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version TEXT PRIMARY KEY,
	applied_at TEXT NOT NULL
);`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Slice(names, func(i, j int) bool {
		ni, erri := migrationNumber(names[i])
		nj, errj := migrationNumber(names[j])
		if erri != nil || errj != nil {
			// Neither error is possible for filenames embedded via go:embed
			// migrations/*.sql — they're checked into the repo, not
			// operator input. Fall back to a lexical compare rather than
			// panicking, so a malformed embedded filename is merely
			// mis-ordered, not fatal.
			return names[i] < names[j]
		}
		return ni < nj
	})

	if len(names) > 0 {
		highestKnown, err := migrationNumber(names[len(names)-1])
		if err != nil {
			return fmt.Errorf("parse embedded migration filename %s: %w", names[len(names)-1], err)
		}
		highestApplied, found, err := highestAppliedMigration(ctx, db)
		if err != nil {
			return err
		}
		if found && highestApplied > highestKnown {
			return fmt.Errorf("%w (database has migration %04d applied, this binary only knows up to %04d)",
				ErrSchemaNewerThanBinary, highestApplied, highestKnown)
		}
	}

	for _, name := range names {
		applied, err := migrationApplied(ctx, db, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := applyMigration(ctx, db, name, string(body)); err != nil {
			return err
		}
	}
	return nil
}

// migrationNumber extracts the leading numeric prefix from a migration
// filename (e.g. "0025_deployment_railpack_artifacts.sql" -> 25), used to
// order and compare migrations by number rather than by raw filename string.
func migrationNumber(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration filename %q has no numeric prefix", name)
	}
	n, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("migration filename %q has a non-numeric prefix: %w", name, err)
	}
	return n, nil
}

// highestAppliedMigration returns the highest migration number recorded in
// schema_migrations, and false if the table has no rows yet (a fresh
// database — not a downgrade, and not an error).
func highestAppliedMigration(ctx context.Context, db *sql.DB) (highest int, found bool, err error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return 0, false, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return 0, false, fmt.Errorf("scan applied migration: %w", err)
		}
		n, err := migrationNumber(version)
		if err != nil {
			return 0, false, fmt.Errorf("parse applied migration %s: %w", version, err)
		}
		if !found || n > highest {
			highest, found = n, true
		}
	}
	if err := rows.Err(); err != nil {
		return 0, false, fmt.Errorf("list applied migrations: %w", err)
	}
	return highest, found, nil
}

// migrationApplied returns true if version (the migration file name) was already applied.
func migrationApplied(ctx context.Context, db *sql.DB, version string) (bool, error) {
	var count int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`,
		version,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return count > 0, nil
}

// applyMigration runs one migration file in a transaction: execute SQL, then insert version.
func applyMigration(ctx context.Context, db *sql.DB, version, sqlText string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start migration %s: %w", version, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, sqlText); err != nil {
		return fmt.Errorf("execute migration %s: %w", version, err)
	}
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations(version, applied_at) VALUES(?, datetime('now'))`,
		version,
	); err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}
	return nil
}
