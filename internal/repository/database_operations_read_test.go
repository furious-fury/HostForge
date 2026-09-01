package repository

import (
	"context"
	"testing"
	"time"
)

// ListDatabaseOperations and GetDatabaseOperation share one column list and
// one scan function. The real risk in collapsing the old select-ids-then-
// fetch-each loop into a single query is a positional-scan mismatch, which
// would silently populate the wrong fields rather than error. Asserting the
// two reads agree field-by-field is what catches that.
func TestListDatabaseOperationsMatchesGetDatabaseOperation(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	app, err := store.CreateApplication(ctx, "Ops", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "18",
		Instances: []CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test",
			NetworkAlias: "database", VolumeName: "hostforge-db-ops", InternalPort: 5432, ResourcePreset: "standard",
			CPULimitMillis: 1000, MemoryLimitBytes: 1024 * 1024 * 1024,
			DatabaseName: "app", Username: "app", PasswordCT: []byte("sealed"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Give every string column a distinct, recognisable value so a swapped
	// pair of same-typed fields lands somewhere visible rather than reading
	// as an empty string on both sides.
	operationID := created.Operations[0].ID
	if _, err := store.ClaimNextDatabaseOperation(ctx, "worker-a", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDatabaseOperation(ctx, operationID, "failed", "creating_volume", 42, "disk_full", "no space left on device"); err != nil {
		t.Fatal(err)
	}

	single, err := store.GetDatabaseOperation(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListDatabaseOperations(ctx, created.Service.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed %d operations, want 1", len(listed))
	}
	if listed[0] != single {
		t.Fatalf("list and get disagree:\n list=%+v\n  get=%+v", listed[0], single)
	}

	// Both reads share scanDatabaseOperation, so comparing them to each other
	// cannot catch a column-order mistake — it would corrupt both identically.
	// Assert the values themselves.
	got := listed[0]
	for _, field := range []struct{ name, got, want string }{
		{"id", got.ID, operationID},
		{"service_id", got.ServiceID, created.Service.ID},
		{"database_instance_id", got.DatabaseInstanceID, created.Instances[0].ID},
		{"operation_type", got.OperationType, "provision"},
		{"status", got.Status, "failed"},
		{"progress_step", got.ProgressStep, "creating_volume"},
		{"error_code", got.ErrorCode, "disk_full"},
		{"error_message", got.ErrorMessage, "no space left on device"},
	} {
		if field.got != field.want {
			t.Errorf("%s = %q, want %q", field.name, field.got, field.want)
		}
	}
	if got.ProgressPercent != 42 {
		t.Errorf("progress_percent = %d, want 42", got.ProgressPercent)
	}
	if got.AttemptCount != 1 {
		t.Errorf("attempt_count = %d, want 1", got.AttemptCount)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() || got.StartedAt.IsZero() || got.CompletedAt.IsZero() {
		t.Errorf("a timestamp column scanned as zero: %+v", got)
	}
}

// The list is ordered newest-first and clamped, and must not leak rows
// belonging to another service.
func TestListDatabaseOperationsOrdersAndScopesRows(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	app, err := store.CreateApplication(ctx, "Ops", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	newDatabase := func(name, alias string) CreatedDatabaseService {
		t.Helper()
		result, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
			ApplicationID: app.ID, Name: name, Engine: "postgresql", DefaultVersion: "18",
			Instances: []CreateDatabaseInstanceInput{{
				EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test",
				NetworkAlias: alias, VolumeName: "hostforge-db-" + alias, InternalPort: 5432, ResourcePreset: "standard",
				CPULimitMillis: 1000, MemoryLimitBytes: 1024 * 1024 * 1024,
				DatabaseName: "app", Username: "app", PasswordCT: []byte("sealed"),
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	target := newDatabase("target", "target")
	other := newDatabase("other", "other")

	listed, err := store.ListDatabaseOperations(ctx, target.Service.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range listed {
		if operation.ServiceID != target.Service.ID {
			t.Fatalf("operation %s belongs to %s, not the requested service", operation.ID, operation.ServiceID)
		}
	}
	for i := 1; i < len(listed); i++ {
		if listed[i-1].CreatedAt.Before(listed[i].CreatedAt) {
			t.Fatalf("operations are not ordered newest-first: %v before %v", listed[i-1].CreatedAt, listed[i].CreatedAt)
		}
	}
	if len(listed) == 0 {
		t.Fatal("expected the provision operation to be listed")
	}
	_ = other
}
