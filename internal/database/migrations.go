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

// ApplyMigrations ensures schema_migrations exists, then runs every pending
// embedded migration, in numeric filename order. See PendingMigrations for
// how "pending" is computed and how a downgrade is rejected. Each migration
// runs in a single transaction: DDL + insert into schema_migrations.
func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	pending, err := pendingMigrations(ctx, db)
	if err != nil {
		return err
	}
	for _, name := range pending {
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

// PendingMigrations returns the embedded migration filenames not yet
// recorded in schema_migrations, in the same numeric order ApplyMigrations
// applies them in. It performs the same downgrade guard ApplyMigrations
// does, so a database written by a newer binary returns
// ErrSchemaNewerThanBinary here too.
//
// Exported so a caller that needs to know "is anything pending" before
// deciding to do something else first — internal/database/sqlite.go's
// pre-migration snapshot — can ask without duplicating this logic, and
// without ApplyMigrations itself growing a parameter every existing direct
// caller (including several tests) would have to start passing.
func PendingMigrations(ctx context.Context, db *sql.DB) ([]string, error) {
	return pendingMigrations(ctx, db)
}

// pendingMigrations ensures schema_migrations exists, reads and numerically
// sorts the embedded migration filenames — the leading digits, not the raw
// string, order them, since a lexical sort would misorder once migration
// counts reach different digit widths (e.g. "10000_x.sql" before
// "9999_x.sql") — rejects a downgrade, and returns exactly the filenames
// not yet recorded in schema_migrations.
//
// Before returning anything, the database's highest already-applied
// migration is compared against the highest migration this binary knows
// about. If the database is ahead, this returns ErrSchemaNewerThanBinary
// instead of touching the database further — a downgrade (running an older
// binary against a newer database) is rejected outright rather than
// silently reporting nothing pending and leaving the mismatch for
// application code to trip over later.
func pendingMigrations(ctx context.Context, db *sql.DB) ([]string, error) {
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version TEXT PRIMARY KEY,
	applied_at TEXT NOT NULL
);`); err != nil {
		return nil, fmt.Errorf("ensure schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
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
			return nil, fmt.Errorf("parse embedded migration filename %s: %w", names[len(names)-1], err)
		}
		highestApplied, found, err := highestAppliedMigration(ctx, db)
		if err != nil {
			return nil, err
		}
		if found && highestApplied > highestKnown {
			return nil, fmt.Errorf("%w (database has migration %04d applied, this binary only knows up to %04d)",
				ErrSchemaNewerThanBinary, highestApplied, highestKnown)
		}
	}

	applied, err := appliedMigrationSet(ctx, db)
	if err != nil {
		return nil, err
	}
	pending := make([]string, 0, len(names))
	for _, name := range names {
		if !applied[name] {
			pending = append(pending, name)
		}
	}
	return pending, nil
}

// appliedMigrationSet returns every version already recorded in
// schema_migrations, as a set — one query, instead of the one
// SELECT-COUNT-per-embedded-file round trip the old migrationApplied helper
// used to cost when called once per name in a loop.
func appliedMigrationSet(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	return applied, nil
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
