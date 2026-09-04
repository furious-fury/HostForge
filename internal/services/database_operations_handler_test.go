package services

import (
	"context"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/furious-fury/HostForge/internal/workers"
)

func newHandlerTestFixture(t *testing.T, alias string) (*repository.Store, repository.CreatedDatabaseService) {
	t.Helper()
	store := newWorkerTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Handlers", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateDatabaseService(ctx, repository.CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: alias, Engine: "postgresql", DefaultVersion: "18",
		Instances: []repository.CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test",
			NetworkAlias: alias, VolumeName: "hostforge-db-" + alias, InternalPort: 5432,
			ResourcePreset: "standard", CPULimitMillis: 1000, MemoryLimitBytes: 1024 * 1024 * 1024,
			DatabaseName: "app", Username: "app", PasswordCT: []byte("sealed"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, created
}

func newTestHandler(t *testing.T, store *repository.Store, operationType string) *databaseOperationHandler {
	t.Helper()
	return &databaseOperationHandler{
		kind:             DatabaseOperationKind(operationType),
		log:              discardLogger(),
		store:            store,
		sealer:           newWorkerTestSealer(t),
		dataDir:          t.TempDir(),
		minFreeDiskBytes: 0,
		cfg:              nil,
	}
}

// Every claimable operation type must have a handler, or the runtime fails
// the operation with operation_kind_not_registered at execution time —
// after it has been claimed, which is the worst place to discover it.
func TestEveryClaimableOperationTypeHasAHandler(t *testing.T) {
	t.Parallel()
	handlers := NewDatabaseOperationHandlers(discardLogger(), nil, nil, nil, "", 0, nil, nil)

	registered := map[string]bool{}
	for _, handler := range handlers {
		registered[handler.Kind()] = true
	}
	// The operation types the CHECK constraint allows, minus the two that are
	// never claimed: 'delete' is written terminal as an audit record, and
	// 'purge' has no writer at all.
	for _, operationType := range []string{
		"provision", "start", "stop", "restart", "backup", "restore",
		"rotate_credentials", "upgrade", "restore_deleted",
	} {
		if !registered[DatabaseOperationKind(operationType)] {
			t.Errorf("no handler registered for %s", DatabaseOperationKind(operationType))
		}
	}
	if registered["db_delete"] || registered["db_purge"] {
		t.Error("a handler is registered for an operation type that is never claimed")
	}
}

// Gate one, previously a predicate inside the claim query: work for an
// instance being deleted is obsolete. The old behaviour left it queued
// forever, invisible to the claim and polled by the UI indefinitely.
func TestReadyReportsObsoleteForDeletedInstance(t *testing.T) {
	t.Parallel()
	store, created := newHandlerTestFixture(t, "obsolete")
	ctx := context.Background()

	if _, err := store.UpdateDatabaseInstanceState(ctx, created.Instances[0].ID,
		repository.UpdateDatabaseInstanceStateInput{DesiredState: "deleted", Status: "stopping"}); err != nil {
		t.Fatal(err)
	}

	handler := newTestHandler(t, store, "provision")
	operation, err := store.GetOperation(ctx, created.Operations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Ready(ctx, operation); err != workers.ErrOperationObsolete {
		t.Fatalf("Ready = %v, want ErrOperationObsolete", err)
	}
}

func TestReadyAllowsHealthyInstance(t *testing.T) {
	t.Parallel()
	store, created := newHandlerTestFixture(t, "healthy")
	ctx := context.Background()

	if _, err := store.UpdateDatabaseInstanceState(ctx, created.Instances[0].ID,
		repository.UpdateDatabaseInstanceStateInput{
			DockerContainerID: "container-1", DesiredState: "running", Status: "healthy",
		}); err != nil {
		t.Fatal(err)
	}

	handler := newTestHandler(t, store, "provision")
	operation, err := store.GetOperation(ctx, created.Operations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	retryAfter, err := handler.Ready(ctx, operation)
	if err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if retryAfter != 0 {
		t.Fatalf("retryAfter = %v, want 0 for a healthy instance", retryAfter)
	}
}

// Gate two: a restore waits for a healthy target rather than failing
// against one that is still provisioning.
func TestReadyDefersRestoreUntilTargetIsHealthy(t *testing.T) {
	t.Parallel()
	store, created := newHandlerTestFixture(t, "restoretarget")
	ctx := context.Background()
	handler := newTestHandler(t, store, "restore")
	instanceID := created.Instances[0].ID

	// Straight from the fixture the instance is still provisioning, so a
	// restore against it must wait rather than run.
	retryAfter, err := handler.restoreReadiness(ctx, repository.DatabaseOperation{
		ID: created.Operations[0].ID, OperationType: "restore", DatabaseInstanceID: instanceID,
	})
	if err != nil {
		t.Fatalf("restoreReadiness: %v", err)
	}
	if retryAfter != databaseRestoreRetryAfter {
		t.Fatalf("retryAfter = %v, want %v while the target is not healthy", retryAfter, databaseRestoreRetryAfter)
	}

	// Once the target is healthy and there is no safety backup to wait on,
	// it becomes runnable.
	if _, err := store.UpdateDatabaseInstanceState(ctx, instanceID,
		repository.UpdateDatabaseInstanceStateInput{
			DockerContainerID: "container-1", DesiredState: "running", Status: "healthy",
		}); err != nil {
		t.Fatal(err)
	}
	retryAfter, err = handler.restoreReadiness(ctx, repository.DatabaseOperation{
		ID: created.Operations[0].ID, OperationType: "restore", DatabaseInstanceID: instanceID,
	})
	// No restore job exists for this operation id, so the lookup fails; what
	// matters is that the healthy check passed rather than deferring.
	if retryAfter == databaseRestoreRetryAfter && err == nil {
		t.Fatal("restore still deferred against a healthy target")
	}
}

// The deferral interval must stay comfortably longer than the claim poll,
// or a waiting restore re-claims itself in a tight loop.
func TestRestoreRetryIntervalIsLongerThanTheClaimPoll(t *testing.T) {
	t.Parallel()
	if databaseRestoreRetryAfter <= 2*time.Second {
		t.Fatalf("databaseRestoreRetryAfter = %v, which is not longer than the 2s claim poll",
			databaseRestoreRetryAfter)
	}
}
