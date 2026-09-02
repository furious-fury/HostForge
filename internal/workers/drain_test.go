package workers

import (
	"context"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/repository"
)

// A non-resumable operation (max_attempts=1, the shape deploys use) must be
// recorded as failed/interrupted on a timed-out drain, not requeued.
// Deferring gives the attempt back — that is DeferOperation's whole
// point — so a max_attempts=1 operation deferred here would be reclaimed and
// re-run on the next boot, which is exactly what "no deploy resume" forbids.
func TestRuntimeWaitFailsNonResumableOperationPastTheDeadline(t *testing.T) {
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
	if _, err := h.store.EnqueueOperation(context.Background(), repository.NewOperationInput{
		ID: "op-1", Kind: "test_kind", LockKey: "k1", MaxAttempts: 1,
	}); err != nil {
		t.Fatal(err)
	}
	h.start()

	<-started
	h.cancel()

	drainCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := h.runtime.Wait(drainCtx); err == nil {
		t.Fatal("Wait returned nil, want a deadline error")
	}

	operation, err := h.store.GetOperation(context.Background(), "op-1")
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != "failed" {
		t.Fatalf("status = %q, want failed: a non-resumable operation must not be requeued on drain timeout", operation.Status)
	}
	if operation.ErrorCode != "interrupted" {
		t.Fatalf("error_code = %q, want interrupted", operation.ErrorCode)
	}
	if operation.LeaseOwner != "" || operation.CompletedAt.IsZero() {
		t.Fatalf("operation not terminal: %+v", operation)
	}
	// Not re-run: since it's terminal, a boot recovery pass would leave it
	// alone rather than picking it back up.
	if operation.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1 (unchanged — it was not given back)", operation.Attempt)
	}
}

// A resumable operation (max_attempts > 1) keeps the existing defer
// behaviour: requeued so the next start recovers it immediately.
func TestRuntimeWaitDefersResumableOperationPastTheDeadline(t *testing.T) {
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
	if _, err := h.store.EnqueueOperation(context.Background(), repository.NewOperationInput{
		ID: "op-1", Kind: "test_kind", LockKey: "k1", MaxAttempts: 5,
	}); err != nil {
		t.Fatal(err)
	}
	h.start()

	<-started
	h.cancel()

	drainCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := h.runtime.Wait(drainCtx); err == nil {
		t.Fatal("Wait returned nil, want a deadline error")
	}

	operation, err := h.store.GetOperation(context.Background(), "op-1")
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != "queued" {
		t.Fatalf("status = %q, want queued: a resumable operation should be requeued, not failed", operation.Status)
	}
	if operation.LeaseOwner != "" {
		t.Fatalf("lease_owner = %q, want released", operation.LeaseOwner)
	}
}
