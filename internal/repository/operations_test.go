package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func enqueueTestOperation(t *testing.T, store *Store, id, lockKey string, priority int) Operation {
	t.Helper()
	operation, err := store.EnqueueOperation(context.Background(), NewOperationInput{
		ID: id, Kind: "test_kind", LockKey: lockKey, Priority: priority, ServiceID: "svc-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return operation
}

func claimTestOperation(t *testing.T, store *Store, owner string) (Operation, error) {
	t.Helper()
	return store.ClaimNextOperation(context.Background(), ClaimOptions{Owner: owner, Lease: time.Minute})
}

// The reason lock_key exists. Work sharing a key runs one at a time, which
// is the invariant the enqueue-time COUNT(*) guards only appeared to
// provide — four of the seven insert sites bypassed them.
func TestClaimSerialisesOperationsSharingALockKey(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	first := enqueueTestOperation(t, store, "op-1", "dbi:inst-1", 0)
	enqueueTestOperation(t, store, "op-2", "dbi:inst-1", 0)

	claimed, err := claimTestOperation(t, store, "worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != first.ID {
		t.Fatalf("claimed %s, want the older %s", claimed.ID, first.ID)
	}

	if _, err := claimTestOperation(t, store, "worker-b"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second claim on a held lock key returned %v, want sql.ErrNoRows", err)
	}

	if err := store.CompleteOperation(ctx, CompleteOperationInput{
		ID: first.ID, Owner: "worker-a", Status: "success",
	}); err != nil {
		t.Fatal(err)
	}

	second, err := claimTestOperation(t, store, "worker-b")
	if err != nil {
		t.Fatalf("claim after the lock was released: %v", err)
	}
	if second.ID != "op-2" {
		t.Fatalf("claimed %s, want op-2", second.ID)
	}
}

// Without this, a bug that serialises everything globally still passes the
// test above and nobody notices until throughput collapses in production.
func TestClaimRunsDistinctLockKeysConcurrently(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	enqueueTestOperation(t, store, "op-1", "dbi:inst-1", 0)
	enqueueTestOperation(t, store, "op-2", "dbi:inst-2", 0)

	if _, err := claimTestOperation(t, store, "worker-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := claimTestOperation(t, store, "worker-b"); err != nil {
		t.Fatalf("claim on an unrelated lock key was blocked: %v", err)
	}
}

func TestClaimOrdersByPriorityThenCreatedAt(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	// Distinct lock keys so ordering, not serialisation, decides.
	enqueueTestOperation(t, store, "op-low", "k1", 50)
	enqueueTestOperation(t, store, "op-mid", "k2", 100)
	enqueueTestOperation(t, store, "op-high", "k3", 200)

	for i, want := range []string{"op-high", "op-mid", "op-low"} {
		claimed, err := claimTestOperation(t, store, "worker-a")
		if err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		if claimed.ID != want {
			t.Fatalf("claim %d returned %s, want %s", i+1, claimed.ID, want)
		}
	}
}

func TestClaimSkipsOperationsScheduledInTheFuture(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	operation := enqueueTestOperation(t, store, "op-1", "k1", 0)

	// Claim, then defer it well into the future.
	if _, err := claimTestOperation(t, store, "worker-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeferOperation(ctx, operation.ID, "worker-a", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := claimTestOperation(t, store, "worker-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claimed an operation scheduled in the future: %v", err)
	}

	// Move it into the past and it becomes claimable again.
	if _, err := store.db.ExecContext(ctx, `UPDATE operations SET available_at=? WHERE id=?`,
		time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), operation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := claimTestOperation(t, store, "worker-a"); err != nil {
		t.Fatalf("operation did not become claimable once its schedule passed: %v", err)
	}
}

// A deferral means "not yet", not "tried and failed". Without the attempt
// compensation an operation waiting on a dependency burns through
// max_attempts and fails for being patient. This test is what stops a future
// reader deleting the MAX(attempt-1,0) as a curiosity.
func TestDeferredOperationDoesNotConsumeAnAttempt(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	operation := enqueueTestOperation(t, store, "op-1", "k1", 0)

	for i := 0; i < 3; i++ {
		if _, err := claimTestOperation(t, store, "worker-a"); err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		if err := store.DeferOperation(ctx, operation.ID, "worker-a", 0); err != nil {
			t.Fatalf("defer %d: %v", i+1, err)
		}
	}

	claimed, err := claimTestOperation(t, store, "worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Attempt != 1 {
		t.Fatalf("attempt = %d after three deferrals and one real claim, want 1", claimed.Attempt)
	}
}

func TestDeferOperationRequiresTheLeaseHolder(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	operation := enqueueTestOperation(t, store, "op-1", "k1", 0)
	if _, err := claimTestOperation(t, store, "worker-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeferOperation(ctx, operation.ID, "worker-b", time.Second); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("a non-holder deferred the operation: %v", err)
	}
}

func TestRenewOperationLeaseReportsCancellation(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	operation := enqueueTestOperation(t, store, "op-1", "k1", 0)
	if _, err := claimTestOperation(t, store, "worker-a"); err != nil {
		t.Fatal(err)
	}

	cancelRequested, err := store.RenewOperationLease(ctx, operation.ID, "worker-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if cancelRequested {
		t.Fatal("cancellation reported before it was requested")
	}

	if changed, err := store.RequestOperationCancellation(ctx, operation.ID); err != nil || !changed {
		t.Fatalf("RequestOperationCancellation: changed=%v err=%v", changed, err)
	}
	cancelRequested, err = store.RenewOperationLease(ctx, operation.ID, "worker-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !cancelRequested {
		t.Fatal("cancellation request not reported to the lease holder")
	}

	// A worker that no longer holds the lease is told so.
	if _, err := store.RenewOperationLease(ctx, operation.ID, "worker-b", time.Minute); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("renewal by a non-holder returned %v, want sql.ErrNoRows", err)
	}
}

func TestCompleteOperationPreservesProgressStepWhenBlank(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	operation := enqueueTestOperation(t, store, "op-1", "k1", 0)
	if _, err := claimTestOperation(t, store, "worker-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE operations SET progress_step='restoring_volume' WHERE id=?`, operation.ID); err != nil {
		t.Fatal(err)
	}

	if err := store.CompleteOperation(ctx, CompleteOperationInput{
		ID: operation.ID, Owner: "worker-a", Status: "cancelled",
	}); err != nil {
		t.Fatal(err)
	}
	completed, err := store.GetOperation(ctx, operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", completed.Status)
	}
	if completed.ProgressStep != "restoring_volume" {
		t.Fatalf("progress_step = %q, want the step the handler last reported", completed.ProgressStep)
	}
	if completed.LeaseOwner != "" || completed.CompletedAt.IsZero() {
		t.Fatalf("lease not released or completion not stamped: %+v", completed)
	}
}

func TestRecoveryRequeuesRunningOperations(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	operation := enqueueTestOperation(t, store, "op-1", "k1", 0)
	if _, err := claimTestOperation(t, store, "worker-a"); err != nil {
		t.Fatal(err)
	}

	// Note the lease is still live: recovery runs at boot, when there is by
	// definition no worker holding it, so it does not wait for expiry.
	requeued, failed, err := store.RecoverOperations(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if requeued != 1 || failed != 0 {
		t.Fatalf("recovered requeued=%d failed=%d, want 1 and 0", requeued, failed)
	}

	recovered, err := store.GetOperation(ctx, operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "queued" || recovered.LeaseOwner != "" || !recovered.LeaseExpiresAt.IsZero() {
		t.Fatalf("operation not returned to the queue: %+v", recovered)
	}
	if recovered.Attempt != 1 {
		t.Fatalf("attempt = %d, want the claim to still be counted", recovered.Attempt)
	}
	if recovered.ProgressStep != "recovery" {
		t.Fatalf("progress_step = %q, want recovery", recovered.ProgressStep)
	}
}

func TestRecoveryFailsOperationsPastMaxAttempts(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	operation, err := store.EnqueueOperation(ctx, NewOperationInput{
		ID: "op-1", Kind: "test_kind", LockKey: "k1", MaxAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claimTestOperation(t, store, "worker-a"); err != nil {
		t.Fatal(err)
	}

	requeued, failed, err := store.RecoverOperations(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if requeued != 0 || failed != 1 {
		t.Fatalf("recovered requeued=%d failed=%d, want 0 and 1", requeued, failed)
	}

	recovered, err := store.GetOperation(ctx, operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "failed" || recovered.ErrorCode != "interrupted" {
		t.Fatalf("exhausted operation not failed: %+v", recovered)
	}
	if recovered.CompletedAt.IsZero() {
		t.Fatal("failed operation has no completion timestamp")
	}
}

// An operation at its attempt limit must not be claimable, or the runtime
// spins on a row it can never finish.
func TestClaimSkipsOperationsPastMaxAttempts(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.EnqueueOperation(ctx, NewOperationInput{
		ID: "op-1", Kind: "test_kind", LockKey: "k1", MaxAttempts: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := claimTestOperation(t, store, "worker-a"); err != nil {
		t.Fatal(err)
	}
	// Return it to the queue with its attempt spent.
	if _, err := store.db.ExecContext(ctx,
		`UPDATE operations SET status='queued',lease_owner='',lease_expires_at='' WHERE id='op-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := claimTestOperation(t, store, "worker-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claimed an operation with no attempts left: %v", err)
	}
}

func TestClaimHonoursMinPriority(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	enqueueTestOperation(t, store, "op-low", "k1", 50)

	if _, err := store.ClaimNextOperation(context.Background(), ClaimOptions{
		Owner: "reserved-worker", Lease: time.Minute, MinPriority: 150,
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("a priority-reserved worker claimed low-priority work: %v", err)
	}
	if _, err := claimTestOperation(t, store, "worker-a"); err != nil {
		t.Fatalf("an unreserved worker could not claim it: %v", err)
	}
}
