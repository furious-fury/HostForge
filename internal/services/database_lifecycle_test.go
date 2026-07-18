package services

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/hostforge/hostforge/internal/database"
	"github.com/hostforge/hostforge/internal/repository"
)

func TestDeleteDatabaseServiceRetainsVolumeMetadata(t *testing.T) {
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, filepath.Join(t.TempDir(), "hostforge.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)
	app, err := store.CreateApplication(ctx, "Database app", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, err := store.CreateDatabaseService(ctx, repository.CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "postgres", Engine: "postgresql", DefaultVersion: "18",
		Instances: []repository.CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test",
			NetworkAlias: "postgres", InternalPort: 5432, VolumeName: "hostforge-db-safe",
			ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024,
			DatabaseName: "app", Username: "app", PasswordCT: []byte{1},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	result, err := DeleteDatabaseServiceAndRuntime(ctx, log, nil, store, nil, created.Service.ID, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Instances) != 1 || result.Instances[0].VolumeName != "hostforge-db-safe" {
		t.Fatalf("volume metadata lost: %+v", result)
	}
	if result.PurgeAfter.Before(time.Now().UTC().Add(6 * 24 * time.Hour)) {
		t.Fatalf("retention shorter than expected: %s", result.PurgeAfter)
	}
	if _, err := store.GetService(ctx, created.Service.ID); err != nil {
		t.Fatalf("shared service identity must remain recoverable: %v", err)
	}
	operation, err := store.GetDatabaseOperation(ctx, created.Operations[0].ID)
	if err != nil || operation.Status != "cancelled" || operation.ProgressStep != "cancelled_by_delete" {
		t.Fatalf("queued provisioning operation survived deletion: %+v err=%v", operation, err)
	}
}

func TestDeleteDatabaseServiceRejectsRunningOperation(t *testing.T) {
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, filepath.Join(t.TempDir(), "hostforge.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)
	app, _ := store.CreateApplication(ctx, "Busy database", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	created, err := store.CreateDatabaseService(ctx, repository.CreateDatabaseServiceInput{ApplicationID: app.ID, Name: "postgres", Engine: "postgresql", DefaultVersion: "18", Instances: []repository.CreateDatabaseInstanceInput{{EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test", NetworkAlias: "postgres", InternalPort: 5432, VolumeName: "hostforge-db-busy", ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024, DatabaseName: "app", Username: "app", PasswordCT: []byte{1}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDatabaseOperation(ctx, created.Operations[0].ID, "running", "image_pull", 40, "", ""); err != nil {
		t.Fatal(err)
	}
	_, err = DeleteDatabaseServiceAndRuntime(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, store, nil, created.Service.ID, "operator")
	if err == nil || PublicCode(err) != "database_operation_in_progress" {
		t.Fatalf("running database operation was not protected: %v", err)
	}
	instance, _ := store.GetDatabaseInstance(ctx, created.Instances[0].ID)
	if instance.Status == "deleted" || !instance.DeletedAt.IsZero() {
		t.Fatalf("busy database was partially deleted: %+v", instance)
	}
}
