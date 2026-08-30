package database

import (
	"context"
	"errors"
	"testing"
)

// PendingMigrations is the shared source of truth OpenSQLite's pre-migration
// snapshot gate and ApplyMigrations' apply loop both build on — these tests
// exercise it directly, against openTestDB/embeddedMigrationNames from
// migration_downgrade_test.go.

func TestPendingMigrationsOnFreshDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDB(t, "pending-fresh.db")

	pending, err := PendingMigrations(ctx, db)
	if err != nil {
		t.Fatalf("PendingMigrations on fresh database: %v", err)
	}

	want := embeddedMigrationNames(t)
	if len(pending) != len(want) {
		t.Fatalf("pending = %v (%d), want all %d embedded migrations", pending, len(pending), len(want))
	}
	for i, name := range want {
		if pending[i] != name {
			t.Fatalf("pending[%d] = %q, want %q (order must match embeddedMigrationNames)", i, pending[i], name)
		}
	}
}

func TestPendingMigrationsAfterFullyApplied(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDB(t, "pending-applied.db")

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	pending, err := PendingMigrations(ctx, db)
	if err != nil {
		t.Fatalf("PendingMigrations after full apply: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %v, want none after ApplyMigrations already ran", pending)
	}
}

func TestPendingMigrationsRejectsDowngrade(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDB(t, "pending-downgrade.db")

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("initial ApplyMigrations: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, applied_at) VALUES(?, datetime('now'))`,
		"0099_future_fake.sql",
	); err != nil {
		t.Fatalf("seed future migration row: %v", err)
	}

	pending, err := PendingMigrations(ctx, db)
	if err == nil {
		t.Fatalf("PendingMigrations: expected error for downgrade, got pending=%v", pending)
	}
	if !errors.Is(err, ErrSchemaNewerThanBinary) {
		t.Fatalf("PendingMigrations error = %v, want it to wrap ErrSchemaNewerThanBinary", err)
	}
}
