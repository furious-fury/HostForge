package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func newDatabaseOperationFixture(t *testing.T, store *Store, alias string) CreatedDatabaseService {
	t.Helper()
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Attempts", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: alias, Engine: "postgresql", DefaultVersion: "18",
		Instances: []CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test",
			NetworkAlias: alias, VolumeName: "hostforge-db-" + alias, InternalPort: 5432,
			ResourcePreset: "standard", CPULimitMillis: 1000, MemoryLimitBytes: 1024 * 1024 * 1024,
			DatabaseName: "app", Username: "app", PasswordCT: []byte("sealed"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

// An operation that wedges its worker is re-claimed once its lease expires.
// Without a cap that repeats forever: attempt_count climbs, the operation
// never reaches a terminal status, and the UI polls it every 2 seconds
// indefinitely (ADR-0002 §4.3).
func TestExhaustedDatabaseOperationIsNotReclaimed(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	created := newDatabaseOperationFixture(t, store, "wedged")
	operationID := created.Operations[0].ID

	// Claim and let the lease expire, repeatedly — exactly what a worker that
	// dies mid-operation produces.
	for i := 0; i < MaxDatabaseOperationAttempts; i++ {
		claimed, err := store.ClaimNextDatabaseOperation(ctx, "worker-a", time.Second)
		if err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		if claimed.ID != operationID {
			t.Fatalf("claim %d returned %s, want %s", i+1, claimed.ID, operationID)
		}
		if claimed.AttemptCount != i+1 {
			t.Fatalf("claim %d: attempt_count = %d, want %d", i+1, claimed.AttemptCount, i+1)
		}
		expireDatabaseOperationLease(t, store, operationID)
	}

	if _, err := store.ClaimNextDatabaseOperation(ctx, "worker-a", time.Second); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claim past the cap returned %v, want sql.ErrNoRows", err)
	}
}

// Skipping the operation in the claim stops the spin but leaves it queued.
// The sweeper is what turns it into a visible failure.
func TestFailExhaustedDatabaseOperationsMarksInterrupted(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	created := newDatabaseOperationFixture(t, store, "exhausted")
	operationID := created.Operations[0].ID

	for i := 0; i < MaxDatabaseOperationAttempts; i++ {
		if _, err := store.ClaimNextDatabaseOperation(ctx, "worker-a", time.Second); err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		expireDatabaseOperationLease(t, store, operationID)
	}

	count, err := store.FailExhaustedDatabaseOperations(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("failed %d operations, want 1", count)
	}

	operation, err := store.GetDatabaseOperation(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != "failed" {
		t.Errorf("status = %q, want failed", operation.Status)
	}
	if operation.ErrorCode != "interrupted" {
		t.Errorf("error_code = %q, want interrupted", operation.ErrorCode)
	}
	if operation.ProgressStep != "interrupted" {
		t.Errorf("progress_step = %q, want interrupted", operation.ProgressStep)
	}
	if operation.LeaseOwner != "" || !operation.LeaseExpiresAt.IsZero() {
		t.Errorf("lease not cleared: owner=%q expires=%v", operation.LeaseOwner, operation.LeaseExpiresAt)
	}
	if operation.CompletedAt.IsZero() {
		t.Error("completed_at not set on a terminal operation")
	}

	// Idempotent: a second sweep finds nothing, because the first moved the
	// row out of ('queued','running').
	again, err := store.FailExhaustedDatabaseOperations(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("second sweep failed %d operations, want 0", again)
	}
}

// An operation below the cap must be left completely alone — this is the
// assertion that stops the sweeper from failing healthy work.
func TestFailExhaustedDatabaseOperationsLeavesLiveOperations(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	created := newDatabaseOperationFixture(t, store, "healthy")
	operationID := created.Operations[0].ID

	if _, err := store.ClaimNextDatabaseOperation(ctx, "worker-a", time.Minute); err != nil {
		t.Fatal(err)
	}

	count, err := store.FailExhaustedDatabaseOperations(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("swept %d operations, want 0", count)
	}
	operation, err := store.GetDatabaseOperation(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != "running" {
		t.Fatalf("status = %q, want running", operation.Status)
	}
}

// expireDatabaseOperationLease backdates a running operation's lease so the
// next claim treats it as abandoned, standing in for a worker that died.
func expireDatabaseOperationLease(t *testing.T, store *Store, operationID string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(),
		`UPDATE database_operations SET lease_expires_at=? WHERE id=?`,
		time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano), operationID); err != nil {
		t.Fatal(err)
	}
}
