package workers

import (
	"context"
	"testing"
	"time"
)

// ADR-0002 §4.4: cancellation is cooperative and lands between units of
// work, never mid-step. The handler here does two units and checks the
// context at the boundary between them; the assertion is that it saw the
// cancellation there, and that the progress it had already reported
// survives into the terminal row.
func TestCancellationLandsAtAStepBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	firstStepDone := make(chan struct{})
	secondStepRan := make(chan struct{}, 1)

	handler := newRecordingHandler()
	handler.handle = func(handlerCtx context.Context, op Operation) error {
		// Step one always completes.
		close(firstStepDone)

		// Boundary: wait for the cancellation to be observed, which happens
		// on the lease-renewal tick.
		select {
		case <-handlerCtx.Done():
			return handlerCtx.Err()
		case <-time.After(5 * time.Second):
		}

		// Step two must not run.
		secondStepRan <- struct{}{}
		return nil
	}

	h := newTestRuntime(t, handler, withLeaseRefresh(20*time.Millisecond))
	operation := h.enqueue("op-1", "k1")
	h.start()

	<-firstStepDone

	// Record a progress step, as a real handler would mid-operation, so the
	// test can prove it is preserved rather than blanked on cancellation.
	h.exec(`UPDATE operations SET progress_step='restoring_volume',progress_percent=40 WHERE id=?`, operation.ID)
	if err := h.store.RequestOperationCancellation(ctx, operation.ID); err != nil {
		t.Fatal(err)
	}

	cancelled := h.awaitStatus(operation.ID, "cancelled")

	select {
	case <-secondStepRan:
		t.Fatal("the handler ran its second step after cancellation was requested")
	default:
	}
	if cancelled.ProgressStep != "restoring_volume" {
		t.Fatalf("progress_step = %q, want the step the handler had reached", cancelled.ProgressStep)
	}
	if cancelled.LeaseOwner != "" {
		t.Fatalf("lease not released on cancellation: %+v", cancelled)
	}
}

// An operation whose target is gone is cancelled, not failed: nothing went
// wrong. Before this, the database claim query filtered these out in SQL,
// which left them queued forever and polled by the UI indefinitely.
func TestObsoleteOperationIsCancelledNotFailed(t *testing.T) {
	t.Parallel()
	handler := newRecordingHandler()
	handler.ready = func(context.Context, Operation) (time.Duration, error) {
		return 0, ErrOperationObsolete
	}
	h := newTestRuntime(t, nil)
	if err := h.runtime.Register(readinessHandler{recordingHandler: handler}); err != nil {
		t.Fatal(err)
	}
	h.enqueue("op-1", "k1")
	h.start()

	operation := h.awaitStatus("op-1", "cancelled")
	if operation.ProgressStep != "obsolete" {
		t.Fatalf("progress_step = %q, want obsolete", operation.ProgressStep)
	}
	if handler.calls.Load() != 0 {
		t.Fatalf("handler ran %d times for an obsolete operation, want 0", handler.calls.Load())
	}
}

// The readiness hook is what replaces the domain predicates that used to sit
// inside the claim SQL. Deferring must not consume attempts, or an operation
// waiting on a dependency eventually fails for waiting.
func TestNotReadyOperationIsDeferredWithoutConsumingAttempts(t *testing.T) {
	t.Parallel()
	handler := newRecordingHandler()
	var deferrals int
	handler.ready = func(context.Context, Operation) (time.Duration, error) {
		deferrals++
		if deferrals <= 3 {
			return 10 * time.Millisecond, nil
		}
		return 0, nil
	}
	h := newTestRuntime(t, nil)
	if err := h.runtime.Register(readinessHandler{recordingHandler: handler}); err != nil {
		t.Fatal(err)
	}
	h.enqueue("op-1", "k1")
	h.start()

	operation := h.awaitStatus("op-1", "success")
	if operation.Attempt != 1 {
		t.Fatalf("attempt = %d after three deferrals, want 1", operation.Attempt)
	}
	if handler.calls.Load() != 1 {
		t.Fatalf("handler ran %d times, want 1", handler.calls.Load())
	}
}
