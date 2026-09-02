package workers

import (
	"context"
	"testing"
	"time"
)

// A runtime must only claim the kinds it has a handler for, and must process
// its own kind normally while doing so.
//
// This has to be single-runtime and deterministic to be a meaningful
// regression guard. A two-runtime version — each registered for a different
// kind, both started, assert each only handled its own — looks like the more
// direct test of "kind isolation", but it is not: without the claim-side
// kind filter, which of two racing runtimes claims a given row first is
// arbitrary, so that version passes more often than not even with the fix
// removed. Verified: it stayed green with the filter deleted. A single
// runtime with no handler at all for the second kind has no such race —
// without the filter it deterministically claims that operation anyway, and
// fails it as operation_kind_not_registered.
func TestRuntimeOnlyClaimsRegisteredKinds(t *testing.T) {
	t.Parallel()
	store := newSharedTestStore(t)
	handlerA := newRecordingHandler()
	runtimeA := newRuntimeOverStore(t, store, handlerA)

	own := runtimeA.enqueueKind("op-own-kind", "test_kind", "k1")
	other := runtimeA.enqueueKind("op-other-kind", "other_kind", "k2")

	runtimeA.start()
	runtimeA.awaitStatus(own.ID, "success")

	// Give the runtime a few more poll cycles in which it could, if the
	// filter were missing, have claimed the other kind too.
	time.Sleep(100 * time.Millisecond)

	current, err := store.GetOperation(context.Background(), other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "queued" {
		t.Fatalf("other_kind status = %q, want queued: a runtime with no handler for it claimed it anyway", current.Status)
	}
	if ids := handlerA.handledIDs(); len(ids) != 1 || ids[0] != own.ID {
		t.Fatalf("test_kind handler handled %v, want only [%s]", ids, own.ID)
	}

	// A second runtime registered for the left-behind kind picks it up,
	// proving the two-runtime production topology actually completes all
	// work between them rather than starving one kind.
	handlerB := newRecordingHandler()
	handlerB.kind = "other_kind"
	runtimeB := newRuntimeOverStore(t, store, handlerB, withSkipRecovery())
	runtimeB.start()
	runtimeB.awaitStatus(other.ID, "success")

	if ids := handlerB.handledIDs(); len(ids) != 1 || ids[0] != other.ID {
		t.Fatalf("other_kind handler handled %v, want only [%s]", ids, other.ID)
	}
}

// Recovery must have exactly one owner. A second runtime's Start must not
// disturb a claim the first runtime is actively holding.
func TestSkipRecoveryLeavesAnotherRuntimesInFlightClaimUndisturbed(t *testing.T) {
	t.Parallel()
	store := newSharedTestStore(t)

	started := make(chan struct{})
	release := make(chan struct{})
	handlerA := newRecordingHandler()
	handlerA.handle = func(context.Context, Operation) error {
		close(started)
		<-release
		return nil
	}
	runtimeA := newRuntimeOverStore(t, store, handlerA)
	op := runtimeA.enqueueKind("op-a", "test_kind", "k1")
	runtimeA.start()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("runtime A never claimed and started its operation")
	}

	before, err := store.GetOperation(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != "running" || before.LeaseOwner == "" {
		t.Fatalf("setup: op-a = %+v, want running with a lease owner", before)
	}

	// A second runtime, over the same store, starting with recovery skipped.
	handlerB := newRecordingHandler()
	handlerB.kind = "other_kind"
	runtimeB := newRuntimeOverStore(t, store, handlerB, withSkipRecovery())
	runtimeB.start()

	// Give B's Start (and, if the guard were missing, its recovery sweep)
	// time to run before asserting nothing changed.
	time.Sleep(150 * time.Millisecond)

	after, err := store.GetOperation(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != "running" || after.LeaseOwner != before.LeaseOwner {
		t.Fatalf("runtime B's Start disturbed runtime A's in-flight claim: before=%+v after=%+v", before, after)
	}

	close(release)
	runtimeA.awaitStatus(op.ID, "success")
}
