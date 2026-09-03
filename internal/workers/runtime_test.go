package workers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/repository"
)

func TestRuntimeDispatchesToTheRegisteredHandler(t *testing.T) {
	t.Parallel()
	handler := newRecordingHandler()
	h := newTestRuntime(t, handler)
	h.enqueue("op-1", "k1")
	h.start()

	operation := h.awaitStatus("op-1", "success")
	if operation.ProgressPercent != 100 {
		t.Fatalf("progress_percent = %d on success, want 100", operation.ProgressPercent)
	}
	if operation.LeaseOwner != "" || operation.CompletedAt.IsZero() {
		t.Fatalf("lease not released or completion not stamped: %+v", operation)
	}
	if ids := handler.handledIDs(); len(ids) != 1 || ids[0] != "op-1" {
		t.Fatalf("handler saw %v, want [op-1]", ids)
	}
}

func TestRuntimeFailsOperationsWithNoRegisteredHandler(t *testing.T) {
	t.Parallel()
	h := newTestRuntime(t, nil)
	if _, err := h.store.EnqueueOperation(context.Background(), repository.NewOperationInput{
		ID: "op-1", Kind: "unregistered_kind", LockKey: "k1",
	}); err != nil {
		t.Fatal(err)
	}
	h.start()

	operation := h.awaitStatus("op-1", "failed")
	if operation.ErrorCode != "operation_kind_not_registered" {
		t.Fatalf("error_code = %q, want operation_kind_not_registered", operation.ErrorCode)
	}
}

func TestRuntimeRecordsHandlerErrorCode(t *testing.T) {
	t.Parallel()
	handler := newRecordingHandler()
	handler.handle = func(context.Context, Operation) error {
		return Failf("disk_full", "no space left on device")
	}
	h := newTestRuntime(t, handler)
	h.enqueue("op-1", "k1")
	h.start()

	operation := h.awaitStatus("op-1", "failed")
	if operation.ErrorCode != "disk_full" {
		t.Fatalf("error_code = %q, want disk_full", operation.ErrorCode)
	}
	if operation.ErrorMessage != "no space left on device" {
		t.Fatalf("error_message = %q", operation.ErrorMessage)
	}
}

// Recovery is part of Start, so the ordering that used to be load-bearing
// and unwritten cannot be got wrong: there is no second call to sequence.
func TestRecoveryRequeuesRunningOperationsAtBoot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := newRecordingHandler()
	h := newTestRuntime(t, handler)
	operation := h.enqueue("op-1", "k1")

	// Simulate a worker that claimed the operation and died.
	if _, err := h.store.ClaimNextOperation(ctx, repository.ClaimOptions{
		Owner: "dead-worker", Lease: time.Hour,
	}); err != nil {
		t.Fatal(err)
	}

	h.start()

	completed := h.awaitStatus(operation.ID, "success")
	if completed.Attempt != 2 {
		t.Fatalf("attempt = %d, want 2: the abandoned claim should still count", completed.Attempt)
	}

	// The projection must agree, so the existing screens see the same thing.
	databaseOperation, err := h.store.GetDatabaseOperation(ctx, operation.ID)
	if err == nil && databaseOperation.Status != "" && databaseOperation.Status != "success" {
		t.Fatalf("database_operations row is %q while operations is success", databaseOperation.Status)
	}
}

func TestRecoveryFailsOperationsPastMaxAttemptsAtBoot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := newRecordingHandler()
	h := newTestRuntime(t, handler)
	if _, err := h.store.EnqueueOperation(ctx, repository.NewOperationInput{
		ID: "op-1", Kind: "test_kind", LockKey: "k1", MaxAttempts: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.ClaimNextOperation(ctx, repository.ClaimOptions{
		Owner: "dead-worker", Lease: time.Hour,
	}); err != nil {
		t.Fatal(err)
	}

	h.start()

	operation := h.awaitStatus("op-1", "failed")
	if operation.ErrorCode != "interrupted" {
		t.Fatalf("error_code = %q, want interrupted", operation.ErrorCode)
	}
	if handler.calls.Load() != 0 {
		t.Fatalf("handler ran %d times for an exhausted operation, want 0", handler.calls.Load())
	}
}

func TestClaimSerialisesOperationsSharingALockKeyThroughTheRuntime(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	running := make(chan string, 4)
	handler := newRecordingHandler()
	handler.handle = func(ctx context.Context, op Operation) error {
		running <- op.ID
		<-release
		return nil
	}
	h := newTestRuntime(t, handler, withConcurrency(4))
	h.enqueue("op-1", "shared")
	h.enqueue("op-2", "shared")
	h.start()

	select {
	case <-running:
	case <-time.After(5 * time.Second):
		t.Fatal("no operation started")
	}

	// With four workers and two operations sharing a lock key, the second
	// must not start while the first holds it.
	select {
	case id := <-running:
		t.Fatalf("operation %s started while another held the same lock key", id)
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	h.awaitStatus("op-1", "success")
	h.awaitStatus("op-2", "success")
}

func TestRuntimeWaitDrainsInFlightOperations(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	handler := newRecordingHandler()
	handler.handle = func(ctx context.Context, op Operation) error {
		close(started)
		// Respects its context, as a real handler must. That is what makes
		// this test able to tell draining from cancelling: if shutdown
		// cancelled the operation context, this returns context.Canceled and
		// the operation ends cancelled rather than success.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
			return nil
		}
	}
	h := newTestRuntime(t, handler)
	h.enqueue("op-1", "k1")
	h.start()

	<-started
	h.cancel()

	drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.runtime.Wait(drainCtx); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	operation, err := h.store.GetOperation(context.Background(), "op-1")
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != "success" {
		t.Fatalf("status = %q, want success: shutdown interrupted the operation instead of draining it", operation.Status)
	}
}

// Past the drain deadline the process is going to exit anyway. Releasing the
// lease means the next start recovers the operation immediately rather than
// waiting out a lease nobody will renew.
func TestRuntimeWaitReleasesLeasesPastTheDeadline(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	blocked := make(chan struct{})
	handler := newRecordingHandler()
	handler.handle = func(ctx context.Context, op Operation) error {
		close(started)
		<-blocked
		return nil
	}
	h := newTestRuntime(t, handler)
	h.enqueue("op-1", "k1")
	h.start()

	<-started
	h.cancel()

	drainCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := h.runtime.Wait(drainCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait returned %v, want context.DeadlineExceeded", err)
	}

	operation, err := h.store.GetOperation(context.Background(), "op-1")
	if err != nil {
		t.Fatal(err)
	}
	if operation.LeaseOwner != "" {
		t.Fatalf("lease_owner = %q after a timed-out drain, want it released", operation.LeaseOwner)
	}
	if operation.Status != "queued" {
		t.Fatalf("status = %q, want queued so the next start picks it up", operation.Status)
	}

	close(blocked)
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{Concurrency: 1}); err == nil {
		t.Fatal("expected an error without a store")
	}
	if _, err := New(Config{Store: stubStore{}, Concurrency: 0}); err == nil {
		t.Fatal("expected an error for non-positive concurrency")
	}
}

func TestRegisterRejectsDuplicateKinds(t *testing.T) {
	t.Parallel()
	runtime, err := New(Config{Store: stubStore{}, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Register(newRecordingHandler()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Register(newRecordingHandler()); err == nil {
		t.Fatal("expected an error registering a duplicate kind")
	}
}

type stubStore struct{}

func (stubStore) ClaimNextOperation(context.Context, repository.ClaimOptions) (repository.Operation, error) {
	return repository.Operation{}, context.Canceled
}
func (stubStore) RenewOperationLease(context.Context, string, string, time.Duration) (bool, error) {
	return false, nil
}
func (stubStore) OperationCancellationRequested(context.Context, string) (bool, error) {
	return false, nil
}
func (stubStore) DeferOperation(context.Context, string, string, time.Duration) error { return nil }
func (stubStore) CompleteOperation(context.Context, repository.CompleteOperationInput) error {
	return nil
}
func (stubStore) RecoverOperations(context.Context, time.Time) (int64, int64, error) {
	return 0, 0, nil
}
