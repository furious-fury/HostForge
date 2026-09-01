package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestCreateDatabaseServiceReservesIsolatedInstancesTransactionally(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil || len(environments) != 2 {
		t.Fatalf("environments=%d err=%v", len(environments), err)
	}
	api, err := store.CreateService(ctx, CreateServiceInput{
		ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/api.git",
		InternalPort: 3000, HealthCheckPath: "/",
	})
	if err != nil {
		t.Fatal(err)
	}

	instances := make([]CreateDatabaseInstanceInput, 0, len(environments))
	for index, environment := range environments {
		instances = append(instances, CreateDatabaseInstanceInput{
			EnvironmentID: environment.ID, EngineVersion: "17", ImageRef: "postgres@sha256:test",
			NetworkAlias: "payments-db-" + environment.Slug, InternalPort: 5432,
			VolumeName: "hostforge-db-test-" + environment.Slug, ResourcePreset: "development",
			CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024,
			DatabaseName: "payments", Username: "payments_user", PasswordCT: []byte{byte(index + 1), 2, 3},
			Bindings: []CreateDatabaseBindingInput{{ConsumerServiceID: api.ID, VariableKey: "database_url"}},
		})
	}
	created, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "17",
		Actor: "operator", Instances: instances,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Service.ServiceType != "database" || created.Database.Engine != "postgresql" {
		t.Fatalf("unexpected database service: %+v %+v", created.Service, created.Database)
	}
	if len(created.Instances) != 2 || len(created.Bindings) != 2 || len(created.Operations) != 2 {
		t.Fatalf("instances=%d bindings=%d operations=%d", len(created.Instances), len(created.Bindings), len(created.Operations))
	}
	if err := store.DeleteDatabaseBinding(ctx, created.Bindings[0].ID); err != nil {
		t.Fatal(err)
	}
	recreatedBinding, err := store.CreateDatabaseBinding(ctx, created.Bindings[0].DatabaseInstanceID, api.ID, "DATABASE_URL", false)
	if err != nil || recreatedBinding.ConsumerServiceID != api.ID {
		t.Fatalf("database binding was not recreated safely: %+v err=%v", recreatedBinding, err)
	}
	worker, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "worker", RepoURL: "https://github.com/acme/worker.git", InternalPort: 3001, HealthCheckPath: "/"})
	if err != nil {
		t.Fatal(err)
	}
	updatedBinding, err := store.UpdateDatabaseBinding(ctx, recreatedBinding.ID, worker.ID, "worker_database_url", false)
	if err != nil || updatedBinding.ConsumerServiceID != worker.ID || updatedBinding.VariableKey != "WORKER_DATABASE_URL" {
		t.Fatalf("database binding was not updated safely: %+v err=%v", updatedBinding, err)
	}
	if bindings, err := store.ListServiceEnvironments(ctx, created.Service.ID); err != nil || len(bindings) != 0 {
		t.Fatalf("database service must not have deployment bindings: count=%d err=%v", len(bindings), err)
	}
	for _, instance := range created.Instances {
		credential, err := store.GetDatabaseCredentialSealed(ctx, instance.ID)
		if err != nil {
			t.Fatal(err)
		}
		if credential.DatabaseName != "payments" || len(credential.PasswordCT) == 0 {
			t.Fatalf("credential not preserved: %+v", credential)
		}
		operation, err := store.GetDatabaseOperation(ctx, created.Operations[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if operation.Status != "queued" || operation.OperationType != "provision" {
			t.Fatalf("unexpected operation: %+v", operation)
		}
		break
	}
	_, err = store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "conflicting database", Engine: "postgresql", DefaultVersion: "17",
		Instances: []CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "17", ImageRef: "postgres@sha256:test",
			NetworkAlias: "conflicting-database", InternalPort: 5432, VolumeName: "hostforge-db-conflict",
			ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024,
			DatabaseName: "conflict", Username: "conflict", PasswordCT: []byte{9},
			Bindings: []CreateDatabaseBindingInput{{ConsumerServiceID: worker.ID, VariableKey: "WORKER_DATABASE_URL"}},
		}},
	})
	if !errors.Is(err, ErrDatabaseBindingConflict) {
		t.Fatalf("a second database claimed an existing environment/service key: %v", err)
	}

	third, err := store.CreateEnvironment(ctx, app.ID, "QA", "qa", "staging")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetServiceEnvironment(ctx, api.ID, third.ID); err != nil {
		t.Fatalf("application service missing new environment binding: %v", err)
	}
	if _, err := store.GetServiceEnvironment(ctx, created.Service.ID, third.ID); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("database service received an application deployment binding: %v", err)
	}
}

func TestQueueDatabaseUpgradePersistsImmutableImageTransition(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, _ := store.CreateApplication(ctx, "Upgrade", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "18", Instances: []CreateDatabaseInstanceInput{{EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:old", NetworkAlias: "database", InternalPort: 5432, VolumeName: "hostforge-db-upgrade", ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024, DatabaseName: "app", Username: "app", PasswordCT: []byte{1}}}})
	if err != nil {
		t.Fatal(err)
	}
	instance := created.Instances[0]
	_, _ = store.UpdateDatabaseOperation(ctx, created.Operations[0].ID, "success", "ready", 100, "", "")
	instance, err = store.UpdateDatabaseInstanceState(ctx, instance.ID, UpdateDatabaseInstanceStateInput{DockerContainerID: "old-container", DesiredState: "running", Status: "healthy"})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := store.QueueDatabaseUpgrade(ctx, instance.ID, "18", "postgres@sha256:new", "operator")
	if err != nil {
		t.Fatal(err)
	}
	job, err := store.GetDatabaseUpgradeJob(ctx, operation.ID)
	if err != nil || job.PreviousImageRef != instance.ImageRef || job.TargetImageRef != "postgres@sha256:new" || job.EngineVersion != "18" {
		t.Fatalf("unexpected upgrade job: %+v err=%v", job, err)
	}
	// A second upgrade on the same instance is now queued rather than
	// rejected: lock_key serialises them at claim time, so the operator gets
	// their action accepted and executed in order instead of an error. The
	// invariant that matters — only one runs at a time — is asserted by
	// TestClaimSerialisesOperationsSharingALockKey.
	second, err := store.QueueDatabaseUpgrade(ctx, instance.ID, "18", "postgres@sha256:another", "operator")
	if err != nil {
		t.Fatalf("concurrent database upgrade was rejected rather than queued: %v", err)
	}
	if second.Status != "queued" {
		t.Fatalf("second upgrade status = %q, want queued", second.Status)
	}
	queued, err := store.GetOperation(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queued.LockKey != "dbi:"+instance.ID {
		t.Fatalf("second upgrade lock_key = %q, want dbi:%s", queued.LockKey, instance.ID)
	}
	if _, err := store.CommitDatabaseInstanceUpgrade(ctx, instance.ID, instance.ImageRef, job.TargetImageRef, "new-container"); err != nil {
		t.Fatal(err)
	}
	updated, _ := store.GetDatabaseInstance(ctx, instance.ID)
	if updated.ImageRef != job.TargetImageRef || updated.DockerContainerID != "new-container" || updated.Status != "healthy" {
		t.Fatalf("upgrade was not committed atomically: %+v", updated)
	}
}

func TestDatabaseDeletionRetainsVolumesAndCanBeRestored(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Retained", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "18",
		Instances: []CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test",
			NetworkAlias: "database", InternalPort: 5432, VolumeName: "hostforge-db-retained",
			ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024,
			DatabaseName: "app", Username: "app", PasswordCT: []byte{1},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := store.MarkDatabaseServiceDeleted(ctx, created.Service.ID, 7*24*time.Hour, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0].Status != "deleted" || deleted[0].PurgeAfter.IsZero() {
		t.Fatalf("unexpected retained state: %+v", deleted)
	}
	due, err := store.ListDatabaseInstancesDueForPurge(ctx, time.Now().UTC().Add(6*24*time.Hour), 10)
	if err != nil || len(due) != 0 {
		t.Fatalf("database became purgeable too early: count=%d err=%v", len(due), err)
	}
	operations, err := store.RestoreDeletedDatabaseService(ctx, created.Service.ID, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 || operations[0].OperationType != "restore_deleted" {
		t.Fatalf("unexpected restore operations: %+v", operations)
	}
	instance, err := store.GetDatabaseInstance(ctx, created.Instances[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if instance.Status != "provisioning" || instance.DesiredState != "running" || !instance.DeletedAt.IsZero() {
		t.Fatalf("database was not restored: %+v", instance)
	}
}

func TestDatabaseDeletionBlocksWorkBeforeForgettingRuntimeIdentity(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, _ := store.CreateApplication(ctx, "Two phase deletion", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "18", Instances: []CreateDatabaseInstanceInput{{EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test", NetworkAlias: "database", InternalPort: 5432, VolumeName: "hostforge-db-two-phase", ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024, DatabaseName: "app", Username: "app", PasswordCT: []byte{1}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = store.UpdateDatabaseOperation(ctx, created.Operations[0].ID, "success", "ready", 100, "", "")
	_, _ = store.UpdateDatabaseInstanceState(ctx, created.Instances[0].ID, UpdateDatabaseInstanceStateInput{DockerContainerID: "container-to-remove", DesiredState: "running", Status: "healthy"})
	prepared, err := store.BeginDatabaseServiceDeletion(ctx, created.Service.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared) != 1 || prepared[0].DockerContainerID != "container-to-remove" || prepared[0].DesiredState != "deleted" || prepared[0].Status != "stopping" || !prepared[0].DeletedAt.IsZero() {
		t.Fatalf("deletion preparation did not preserve retry information: %+v", prepared)
	}
	if _, err := store.QueueDatabaseInstanceOperation(ctx, prepared[0].ID, "restart", "operator"); err == nil {
		t.Fatal("runtime work was accepted after deletion preparation")
	}
	finalized, err := store.FinalizeDatabaseServiceDeletion(ctx, created.Service.ID, 7*24*time.Hour, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if finalized[0].DockerContainerID != "" || finalized[0].Status != "deleted" || finalized[0].DeletedAt.IsZero() || finalized[0].PurgeAfter.IsZero() {
		t.Fatalf("deletion was not finalized after runtime cleanup: %+v", finalized[0])
	}
}

func TestCreateDatabaseServiceRollsBackInvalidCrossApplicationBinding(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "One", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateApplication(ctx, "Two", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	foreignService, err := store.CreateService(ctx, CreateServiceInput{
		ApplicationID: other.ID, Name: "foreign", RepoURL: "https://github.com/acme/foreign.git",
		InternalPort: 3000, HealthCheckPath: "/",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "17",
		Instances: []CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "17", ImageRef: "postgres@sha256:test",
			NetworkAlias: "database", InternalPort: 5432, VolumeName: "hostforge-db-invalid",
			ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024,
			DatabaseName: "app", Username: "app", PasswordCT: []byte{1},
			Bindings: []CreateDatabaseBindingInput{{ConsumerServiceID: foreignService.ID, VariableKey: "DATABASE_URL"}},
		}},
	})
	if !errors.Is(err, ErrInvalidDatabaseBinding) {
		t.Fatalf("expected invalid binding, got %v", err)
	}
	services, err := store.ListApplicationServices(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 0 {
		t.Fatalf("transaction left partial database service: %+v", services)
	}
}

func TestCredentialRotationQueuePreservesHealthyRuntimeState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, _ := store.CreateApplication(ctx, "Rotation", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "18",
		Instances: []CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test",
			NetworkAlias: "database", InternalPort: 5432, VolumeName: "hostforge-db-rotation",
			ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024,
			DatabaseName: "app", Username: "app", PasswordCT: []byte("old-ciphertext"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDatabaseOperation(ctx, created.Operations[0].ID, "success", "ready", 100, "", ""); err != nil {
		t.Fatal(err)
	}
	instance, err := store.UpdateDatabaseInstanceState(ctx, created.Instances[0].ID, UpdateDatabaseInstanceStateInput{
		DockerContainerID: "container", DesiredState: "running", Status: "healthy",
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := store.QueueDatabaseInstanceOperation(ctx, instance.ID, "rotate_credentials", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if operation.OperationType != "rotate_credentials" || operation.Status != "queued" {
		t.Fatalf("unexpected rotation operation: %+v", operation)
	}
	unchanged, _ := store.GetDatabaseInstance(ctx, instance.ID)
	if unchanged.Status != "healthy" || unchanged.DesiredState != "running" {
		t.Fatalf("rotation queue changed runtime state: %+v", unchanged)
	}
	staged, err := store.StageDatabaseCredentialRotation(ctx, instance.ID, []byte("new-ciphertext"))
	if err != nil {
		t.Fatal(err)
	}
	if string(staged.PasswordCT) != "old-ciphertext" || string(staged.PendingPasswordCT) != "new-ciphertext" || staged.PendingCreatedAt.IsZero() {
		t.Fatalf("credential rotation was not durably staged: %+v", staged)
	}
	rotated, err := store.CommitStagedDatabaseCredentialRotation(ctx, instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Generation != 2 || string(rotated.PasswordCT) != "new-ciphertext" || rotated.RotatedAt.IsZero() {
		t.Fatalf("credential rotation was not persisted safely: %+v", rotated)
	}
	if len(rotated.PendingPasswordCT) != 0 || !rotated.PendingCreatedAt.IsZero() {
		t.Fatalf("committed rotation retained pending secret state: %+v", rotated)
	}
}

func TestExpiredDatabaseOperationLeasesAreRequeuedForStartupRecovery(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, _ := store.CreateApplication(ctx, "Recovery", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "18",
		Instances: []CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test",
			NetworkAlias: "database", InternalPort: 5432, VolumeName: "hostforge-db-recovery",
			ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024,
			DatabaseName: "app", Username: "app", PasswordCT: []byte("ciphertext"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDatabaseOperation(ctx, created.Operations[0].ID, "running", "container_start", 65, "", ""); err != nil {
		t.Fatal(err)
	}
	count, err := store.RequeueExpiredDatabaseOperations(ctx, time.Now().UTC())
	if err != nil || count != 1 {
		t.Fatalf("requeued=%d err=%v", count, err)
	}
	operation, _ := store.GetDatabaseOperation(ctx, created.Operations[0].ID)
	if operation.Status != "queued" || operation.ProgressStep != "recovery" || operation.LeaseOwner != "" || !operation.LeaseExpiresAt.IsZero() {
		t.Fatalf("operation was not made recoverable: %+v", operation)
	}
}

func TestDatabaseOperationLeaseCannotBeStolenBeforeExpiry(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, _ := store.CreateApplication(ctx, "Leases", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "18",
		Instances: []CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test",
			NetworkAlias: "database", InternalPort: 5432, VolumeName: "hostforge-db-lease",
			ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024,
			DatabaseName: "app", Username: "app", PasswordCT: []byte("ciphertext"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNextDatabaseOperation(ctx, "worker-a", time.Minute)
	if err != nil || claimed.ID != created.Operations[0].ID || claimed.AttemptCount != 1 || claimed.LeaseOwner != "worker-a" {
		t.Fatalf("unexpected first claim: operation=%+v err=%v", claimed, err)
	}
	if _, err := store.ClaimNextDatabaseOperation(ctx, "worker-b", time.Minute); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("active lease was stolen: %v", err)
	}
	if err := store.RenewDatabaseOperationLease(ctx, claimed.ID, "worker-b", time.Minute); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("wrong owner renewed lease: %v", err)
	}
	if err := store.RenewDatabaseOperationLease(ctx, claimed.ID, "worker-a", time.Minute); err != nil {
		t.Fatalf("lease owner could not renew: %v", err)
	}
}
