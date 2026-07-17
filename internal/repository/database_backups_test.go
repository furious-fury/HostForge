package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestBackupDestinationSecretsStaySealedAndPoliciesProtectDeletion(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	destination, err := store.CreateBackupDestination(ctx, CreateBackupDestinationInput{
		Name: "R2", Provider: "r2", Endpoint: "https://account.r2.cloudflarestorage.com", Region: "auto",
		Bucket: "backups", ObjectPrefix: "hostforge", AccessKeyCT: []byte("sealed-access"), SecretKeyCT: []byte("sealed-secret"),
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListBackupDestinations(ctx)
	if err != nil || len(listed) != 1 || listed[0].ID != destination.ID {
		t.Fatalf("unexpected destinations: %+v err=%v", listed, err)
	}
	sealed, err := store.GetBackupDestinationSealed(ctx, destination.ID)
	if err != nil || string(sealed.AccessKeyCT) != "sealed-access" || string(sealed.SecretKeyCT) != "sealed-secret" {
		t.Fatalf("sealed destination unavailable: %+v err=%v", sealed, err)
	}
	updated, err := store.UpdateBackupDestination(ctx, destination.ID, CreateBackupDestinationInput{Name: "R2 archive", Provider: "r2", Endpoint: "https://account.r2.cloudflarestorage.com", Region: "auto", Bucket: "archive", ObjectPrefix: "daily", AccessKeyCT: []byte("new-sealed-access"), SecretKeyCT: []byte("new-sealed-secret")})
	if err != nil || updated.Name != "R2 archive" || updated.Bucket != "archive" || updated.ObjectPrefix != "daily" {
		t.Fatalf("backup destination was not updated: %+v err=%v", updated, err)
	}
	sealed, _ = store.GetBackupDestinationSealed(ctx, destination.ID)
	if string(sealed.AccessKeyCT) != "new-sealed-access" || string(sealed.SecretKeyCT) != "new-sealed-secret" {
		t.Fatalf("updated destination secrets were not sealed: %+v", sealed)
	}
	s3Destination, err := store.CreateBackupDestination(ctx, CreateBackupDestinationInput{Name: "S3 encrypted", Provider: "s3", Endpoint: "https://s3.example.com", Region: "us-east-1", Bucket: "encrypted", ServerSideEncryption: "aws:kms", SSEKMSKeyID: "alias/hostforge", AccessKeyCT: []byte("sealed-access"), SecretKeyCT: []byte("sealed-secret")})
	if err != nil || s3Destination.ServerSideEncryption != "aws:kms" || s3Destination.SSEKMSKeyID != "alias/hostforge" {
		t.Fatalf("S3 provider-side encryption settings were not persisted: %+v err=%v", s3Destination, err)
	}
	app, _ := store.CreateApplication(ctx, "Backups", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	database, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "18",
		Instances: []CreateDatabaseInstanceInput{{EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test", NetworkAlias: "database", InternalPort: 5432, VolumeName: "hostforge-db-backup", ResourcePreset: "standard", CPULimitMillis: 1000, MemoryLimitBytes: 1024 * 1024 * 1024, DatabaseName: "app", Username: "app", PasswordCT: []byte("sealed")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := store.UpsertDatabaseBackupPolicy(ctx, database.Instances[0].ID, destination.ID, true, "0 2 * * *", "UTC", 30, database.Instances[0].CreatedAt)
	if err != nil || !policy.Enabled || policy.RetentionDays != 30 {
		t.Fatalf("unexpected policy: %+v err=%v", policy, err)
	}
	if err := store.DeleteBackupDestination(ctx, destination.ID); err == nil {
		t.Fatal("destination referenced by a backup policy was deleted")
	}
	if _, err := store.UpdateDatabaseOperation(ctx, database.Operations[0].ID, "success", "ready", 100, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDatabaseInstanceState(ctx, database.Instances[0].ID, UpdateDatabaseInstanceStateInput{DockerContainerID: "container", DesiredState: "running", Status: "healthy"}); err != nil {
		t.Fatal(err)
	}
	backup, operation, err := store.QueueDatabaseBackup(ctx, database.Instances[0].ID, destination.ID, "manual", "operator", 30, 60)
	if err != nil {
		t.Fatal(err)
	}
	if backup.Status != "queued" || backup.OperationID != operation.ID || operation.OperationType != "backup" || operation.Status != "queued" {
		t.Fatalf("backup operation was not queued atomically: backup=%+v operation=%+v", backup, operation)
	}
	linked, err := store.GetDatabaseBackupByOperationID(ctx, operation.ID)
	if err != nil || linked.ID != backup.ID {
		t.Fatalf("backup was not linked to its operation: backup=%+v err=%v", linked, err)
	}
	if err := store.MarkDatabaseBackupRunning(ctx, backup.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDatabaseOperation(ctx, operation.ID, "running", "streaming", 40, "", ""); err != nil {
		t.Fatal(err)
	}
	if count, err := store.RequeueExpiredDatabaseOperations(ctx, time.Now().UTC()); err != nil || count != 1 {
		t.Fatalf("backup recovery requeued=%d err=%v", count, err)
	}
	recovered, err := store.GetDatabaseBackupByOperationID(ctx, operation.ID)
	if err != nil || recovered.Status != "queued" {
		t.Fatalf("expired backup companion state was not recovered: backup=%+v err=%v", recovered, err)
	}
}

func TestTerminalDatabaseBackupCanBeDeletedWhenUnreferenced(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	destination, _ := store.CreateBackupDestination(ctx, CreateBackupDestinationInput{Name: "S3", Provider: "s3", Endpoint: "https://s3.example.com", Region: "test", Bucket: "backups", AccessKeyCT: []byte("access"), SecretKeyCT: []byte("secret")})
	app, _ := store.CreateApplication(ctx, "Delete backup", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, _ := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "18", Instances: []CreateDatabaseInstanceInput{{EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test", NetworkAlias: "database", InternalPort: 5432, VolumeName: "hostforge-db-delete-backup", ResourcePreset: "standard", CPULimitMillis: 1000, MemoryLimitBytes: 1024 * 1024 * 1024, DatabaseName: "app", Username: "app", PasswordCT: []byte("sealed")}}})
	_, _ = store.UpdateDatabaseOperation(ctx, created.Operations[0].ID, "success", "ready", 100, "", "")
	_, _ = store.UpdateDatabaseInstanceState(ctx, created.Instances[0].ID, UpdateDatabaseInstanceStateInput{DockerContainerID: "container", DesiredState: "running", Status: "healthy"})
	backup, operation, err := store.QueueDatabaseBackup(ctx, created.Instances[0].ID, destination.ID, "manual", "operator", 30, 60)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "failed", "failed", 100, "test", "safe failure")
	_, _ = store.CompleteDatabaseBackup(ctx, backup.ID, CompleteDatabaseBackupInput{Status: "failed", ErrorCode: "test", ErrorMessage: "safe failure"})
	prepared, err := store.PrepareDatabaseBackupDeletion(ctx, backup.ID)
	if err != nil || prepared.Status != "failed" {
		t.Fatalf("terminal backup could not be prepared for deletion: %+v err=%v", prepared, err)
	}
	if err := store.DeleteDatabaseBackupRecord(ctx, backup.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDatabaseBackup(ctx, backup.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted backup record still exists: %v", err)
	}
}

func TestDatabaseBackupQueueEnforcesRollingTransferLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	destination, _ := store.CreateBackupDestination(ctx, CreateBackupDestinationInput{Name: "S3", Provider: "s3", Endpoint: "https://s3.example.com", Region: "test", Bucket: "backups", AccessKeyCT: []byte("access"), SecretKeyCT: []byte("secret")})
	app, _ := store.CreateApplication(ctx, "Rate limits", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{ApplicationID: app.ID, Name: "database", Engine: "postgresql", DefaultVersion: "18", Instances: []CreateDatabaseInstanceInput{{EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test", NetworkAlias: "database", InternalPort: 5432, VolumeName: "hostforge-db-rate", ResourcePreset: "standard", CPULimitMillis: 1000, MemoryLimitBytes: 1024 * 1024 * 1024, DatabaseName: "app", Username: "app", PasswordCT: []byte("sealed")}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = store.UpdateDatabaseOperation(ctx, created.Operations[0].ID, "success", "ready", 100, "", "")
	_, _ = store.UpdateDatabaseInstanceState(ctx, created.Instances[0].ID, UpdateDatabaseInstanceStateInput{DockerContainerID: "container", DesiredState: "running", Status: "healthy"})
	_, operation, err := store.QueueDatabaseBackup(ctx, created.Instances[0].ID, destination.ID, "manual", "operator", 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "success", "complete", 100, "", "")
	if _, _, err := store.QueueDatabaseBackup(ctx, created.Instances[0].ID, destination.ID, "manual", "operator", 30, 1); !errors.Is(err, ErrDatabaseTransferLimited) {
		t.Fatalf("second transfer was not rate limited: %v", err)
	}
}

func TestDatabaseRestoreQueueRequiresCompatibleTargetAndSafetyBackup(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	destination, err := store.CreateBackupDestination(ctx, CreateBackupDestinationInput{Name: "S3", Provider: "s3", Endpoint: "https://s3.example.com", Region: "test", Bucket: "backups", AccessKeyCT: []byte("access"), SecretKeyCT: []byte("secret")})
	if err != nil {
		t.Fatal(err)
	}
	app, _ := store.CreateApplication(ctx, "Restore", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	makeDatabase := func(name, volume string) CreatedDatabaseService {
		created, createErr := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{ApplicationID: app.ID, Name: name, Engine: "postgresql", DefaultVersion: "18", Instances: []CreateDatabaseInstanceInput{{EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test", NetworkAlias: name, InternalPort: 5432, VolumeName: volume, ResourcePreset: "standard", CPULimitMillis: 1000, MemoryLimitBytes: 1024 * 1024 * 1024, DatabaseName: name, Username: name, PasswordCT: []byte("sealed"), AdminPasswordCT: []byte("admin")}}})
		if createErr != nil {
			t.Fatal(createErr)
		}
		_, _ = store.UpdateDatabaseOperation(ctx, created.Operations[0].ID, "success", "ready", 100, "", "")
		_, _ = store.UpdateDatabaseInstanceState(ctx, created.Instances[0].ID, UpdateDatabaseInstanceStateInput{DockerContainerID: "container-" + name, DesiredState: "running", Status: "healthy"})
		return created
	}
	source := makeDatabase("source", "restore-source")
	target := makeDatabase("target", "restore-target")
	backup, backupOperation, err := store.QueueDatabaseBackup(ctx, source.Instances[0].ID, destination.ID, "manual", "operator", 30, 60)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = store.UpdateDatabaseOperation(ctx, backupOperation.ID, "success", "complete", 100, "", "")
	if _, err = store.CompleteDatabaseBackup(ctx, backup.ID, CompleteDatabaseBackupInput{Status: "success", ObjectKey: "source.hfbk", ArchiveFormat: "postgresql-custom+gzip+aead", Checksum: "abc", CompressedSize: 10, EncryptionAlgorithm: "AES-256-GCM-CHUNKED", EncryptedDataKey: []byte("wrapped")}); err != nil {
		t.Fatal(err)
	}
	copyOperation, err := store.QueueDatabaseRestore(ctx, backup.ID, target.Instances[0].ID, "", "new_service", "operator", 60)
	if err != nil || copyOperation.OperationType != "restore" {
		t.Fatalf("new-service restore not queued: %+v err=%v", copyOperation, err)
	}
	if _, err := store.PrepareDatabaseBackupDeletion(ctx, backup.ID); err == nil {
		t.Fatal("backup referenced by restore history was prepared for deletion")
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE database_backups SET expires_at=? WHERE id=?`, time.Now().UTC().Add(-time.Hour).Format(time.RFC3339), backup.ID); err != nil {
		t.Fatal(err)
	}
	expired, err := store.ListExpiredDatabaseBackups(ctx, time.Now().UTC(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range expired {
		if candidate.ID == backup.ID {
			t.Fatal("backup referenced by restore history entered retention deletion")
		}
	}
	_, _ = store.UpdateDatabaseOperation(ctx, copyOperation.ID, "cancelled", "cancelled", 0, "", "")
	if _, err := store.QueueDatabaseRestore(ctx, backup.ID, target.Instances[0].ID, "", "replace_current", "operator", 60); err == nil {
		t.Fatal("replace-current restore accepted without a safety backup")
	}
	safety, safetyOperation, err := store.QueueDatabaseBackup(ctx, target.Instances[0].ID, destination.ID, "safety", "operator", 7, 60)
	if err != nil {
		t.Fatal(err)
	}
	replaceOperation, err := store.QueueDatabaseRestore(ctx, backup.ID, target.Instances[0].ID, safety.ID, "replace_current", "operator", 60)
	if err != nil {
		t.Fatal(err)
	}
	job, err := store.GetDatabaseRestoreJob(ctx, replaceOperation.ID)
	if err != nil || job.SafetyBackupID != safety.ID || job.Mode != "replace_current" {
		t.Fatalf("unexpected restore job: %+v err=%v", job, err)
	}
	claimed, err := store.ClaimNextDatabaseOperation(ctx, "worker-a", time.Minute)
	if err != nil || claimed.ID != safetyOperation.ID {
		t.Fatalf("restore ran before its safety backup: claimed=%+v safety=%+v err=%v", claimed, safetyOperation, err)
	}
}
