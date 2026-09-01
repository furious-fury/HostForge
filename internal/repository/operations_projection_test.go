package repository

import (
	"context"
	"testing"
	"time"
)

// assertProjectionConsistent fails if the operations row and its
// database_operations projection disagree on anything they both carry.
func assertProjectionConsistent(t *testing.T, store *Store, id, stage string) {
	t.Helper()
	ctx := context.Background()

	operation, err := store.GetOperation(ctx, id)
	if err != nil {
		t.Fatalf("%s: read operations row: %v", stage, err)
	}
	projected, err := store.GetDatabaseOperation(ctx, id)
	if err != nil {
		t.Fatalf("%s: read database_operations row: %v", stage, err)
	}

	for _, field := range []struct{ name, queue, projection string }{
		{"status", operation.Status, projected.Status},
		{"progress_step", operation.ProgressStep, projected.ProgressStep},
		{"lease_owner", operation.LeaseOwner, projected.LeaseOwner},
		{"error_code", operation.ErrorCode, projected.ErrorCode},
		{"error_message", operation.ErrorMessage, projected.ErrorMessage},
		{"actor", operation.Actor, projected.Actor},
		{"service_id", operation.ServiceID, projected.ServiceID},
	} {
		if field.queue != field.projection {
			t.Errorf("%s: %s diverged — operations=%q database_operations=%q",
				stage, field.name, field.queue, field.projection)
		}
	}
	if operation.ProgressPercent != projected.ProgressPercent {
		t.Errorf("%s: progress_percent diverged — operations=%d database_operations=%d",
			stage, operation.ProgressPercent, projected.ProgressPercent)
	}
	if operation.Attempt != projected.AttemptCount {
		t.Errorf("%s: attempt diverged — operations=%d database_operations=%d",
			stage, operation.Attempt, projected.AttemptCount)
	}
	if operation.CompletedAt.IsZero() != projected.CompletedAt.IsZero() {
		t.Errorf("%s: completed_at diverged — operations=%v database_operations=%v",
			stage, operation.CompletedAt, projected.CompletedAt)
	}
}

// The two tables share a primary key and must never disagree. Divergence is
// the one real risk of the projection design, and it would be invisible: the
// API reads database_operations while the queue reads operations, so a drift
// shows up as a screen that never updates rather than as an error.
//
// This walks a full lifecycle and re-checks after every step.
func TestOperationsAndDatabaseOperationsStayConsistent(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	created := newDatabaseOperationFixture(t, store, "consistent")
	operationID := created.Operations[0].ID

	assertProjectionConsistent(t, store, operationID, "after enqueue")

	if _, err := store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-a", Lease: time.Minute}); err != nil {
		t.Fatal(err)
	}
	assertProjectionConsistent(t, store, operationID, "after claim")

	if _, err := store.RenewOperationLease(ctx, operationID, "worker-a", 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	assertProjectionConsistent(t, store, operationID, "after lease renewal")

	if _, err := store.UpdateDatabaseOperation(ctx, operationID, "running", "creating_volume", 40, "", ""); err != nil {
		t.Fatal(err)
	}
	assertProjectionConsistent(t, store, operationID, "after progress update")

	if err := store.DeferOperation(ctx, operationID, "worker-a", 0); err != nil {
		t.Fatal(err)
	}
	assertProjectionConsistent(t, store, operationID, "after deferral")

	if _, err := store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-b", Lease: time.Minute}); err != nil {
		t.Fatal(err)
	}
	assertProjectionConsistent(t, store, operationID, "after reclaim")

	if err := store.CompleteOperation(ctx, CompleteOperationInput{
		ID: operationID, Owner: "worker-b", Status: "success",
	}); err != nil {
		t.Fatal(err)
	}
	assertProjectionConsistent(t, store, operationID, "after completion")
}

func TestRecoveryKeepsTheProjectionConsistent(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	created := newDatabaseOperationFixture(t, store, "recovered")
	operationID := created.Operations[0].ID

	if _, err := store.ClaimNextOperation(ctx, ClaimOptions{Owner: "dead-worker", Lease: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RecoverOperations(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	assertProjectionConsistent(t, store, operationID, "after recovery")

	// The existing repository test asserts this exact shape on the
	// projection; recovery must keep producing it.
	projected, err := store.GetDatabaseOperation(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if projected.Status != "queued" || projected.ProgressStep != "recovery" ||
		projected.LeaseOwner != "" || !projected.LeaseExpiresAt.IsZero() {
		t.Fatalf("recovery changed the projection's shape: %+v", projected)
	}
}

// Every enqueue path must write both rows. A site that writes only
// database_operations leaves work the queue can never claim.
func TestEveryEnqueuePathWritesBothTables(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	created := newDatabaseOperationFixture(t, store, "paths")
	instanceID := created.Instances[0].ID

	// CreateDatabaseService (site 1) already ran in the fixture.
	if _, err := store.UpdateDatabaseOperation(ctx, created.Operations[0].ID, "success", "ready", 100, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDatabaseInstanceState(ctx, instanceID, UpdateDatabaseInstanceStateInput{
		DockerContainerID: "container-1", DesiredState: "running", Status: "healthy",
	}); err != nil {
		t.Fatal(err)
	}

	// QueueDatabaseInstanceOperation (site 2).
	if _, err := store.QueueDatabaseInstanceOperation(ctx, instanceID, "restart", "operator"); err != nil {
		t.Fatal(err)
	}

	var queueRows, projectionRows int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operations`).Scan(&queueRows); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_operations`).Scan(&projectionRows); err != nil {
		t.Fatal(err)
	}
	if queueRows != projectionRows {
		t.Fatalf("operations has %d rows, database_operations has %d — an enqueue path wrote only one table",
			queueRows, projectionRows)
	}

	// And the ids line up one-to-one, not merely the counts.
	var orphaned int
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM database_operations d
		WHERE NOT EXISTS (SELECT 1 FROM operations o WHERE o.id=d.id)`).Scan(&orphaned); err != nil {
		t.Fatal(err)
	}
	if orphaned != 0 {
		t.Fatalf("%d database_operations rows have no operations row", orphaned)
	}
}
