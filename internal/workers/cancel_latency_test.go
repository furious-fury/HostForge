package workers

import (
	"context"
	"testing"
	"time"
)

// Cancellation used to be observed only when the lease was renewed, so a
// cancelled operation kept working for up to a lease-refresh interval --
// 30 seconds by default. On a real host that was long enough for a
// cancelled deploy to finish its health check and rewrite its own row,
// overwriting the cancellation the operator could already see.
//
// The lease refresh here is set far beyond the test's patience on purpose:
// if cancellation were still tied to it, these would time out rather than
// pass slowly.
func TestCancellationIsObservedWithoutWaitingForTheLeaseRefresh(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	started := make(chan struct{})
	observed := make(chan struct{})

	handler := newRecordingHandler()
	handler.handle = func(handlerCtx context.Context, op Operation) error {
		close(started)
		select {
		case <-handlerCtx.Done():
			close(observed)
			return handlerCtx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	}

	h := newTestRuntime(t, handler,
		withLeaseRefresh(9*time.Second),
		withCancelPoll(20*time.Millisecond))
	operation := h.enqueue("op-1", "k1")
	h.start()

	<-started
	if changed, err := h.store.RequestOperationCancellation(ctx, operation.ID); err != nil || !changed {
		t.Fatalf("RequestOperationCancellation: changed=%v err=%v", changed, err)
	}

	select {
	case <-observed:
	case <-time.After(3 * time.Second):
		t.Fatal("the handler's context was not cancelled promptly; cancellation is still waiting on the lease refresh")
	}

	if got := h.awaitStatus(operation.ID, "cancelled"); got.LeaseOwner != "" {
		t.Fatalf("lease not released on cancellation: %+v", got)
	}
}

// The lease renewal still reports cancellation as well. That path is the
// authority on whether this worker still owns the operation, so it must keep
// stopping the work even if every cancellation poll in between had failed.
func TestLeaseRenewalStillStopsACancelledOperation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	started := make(chan struct{})

	handler := newRecordingHandler()
	handler.handle = func(handlerCtx context.Context, op Operation) error {
		close(started)
		<-handlerCtx.Done()
		return handlerCtx.Err()
	}

	// Cancellation polling effectively disabled; only the lease refresh can
	// notice, which is the pre-fix behaviour.
	h := newTestRuntime(t, handler,
		withLeaseRefresh(30*time.Millisecond),
		withCancelPoll(time.Hour))
	operation := h.enqueue("op-1", "k1")
	h.start()

	<-started
	if _, err := h.store.RequestOperationCancellation(ctx, operation.ID); err != nil {
		t.Fatal(err)
	}
	h.awaitStatus(operation.ID, "cancelled")
}
