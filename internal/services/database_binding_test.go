package services

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/repository"
)

func TestManagedDatabaseBindingInjectsEscapedPrivateConnectionURL(t *testing.T) {
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, filepath.Join(t.TempDir(), "hostforge.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)
	sealer, err := envcrypt.NewFromBase64Key(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	passwordCT, err := sealer.Seal([]byte("p@ss:/word"))
	if err != nil {
		t.Fatal(err)
	}
	app, _ := store.CreateApplication(ctx, "Bindings", "")
	environments, _ := store.ListApplicationEnvironments(ctx, app.ID)
	consumer, err := store.CreateService(ctx, repository.CreateServiceInput{
		ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/api.git",
		InternalPort: 3000, HealthCheckPath: "/",
	})
	if err != nil {
		t.Fatal(err)
	}
	existingValue, err := sealer.Seal([]byte("postgresql://legacy.example/old"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertEnvironmentVariable(ctx, app.ID, environments[0].ID, consumer.ID, "DATABASE_URL", existingValue, "old"); err != nil {
		t.Fatal(err)
	}
	input := repository.CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "postgres", Engine: "postgresql", DefaultVersion: "18",
		Instances: []repository.CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test",
			NetworkAlias: "postgres-production", InternalPort: 5432, VolumeName: "hostforge-db-binding",
			ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024,
			DatabaseName: "payments", Username: "payments_user", PasswordCT: passwordCT,
			Bindings: []repository.CreateDatabaseBindingInput{{ConsumerServiceID: consumer.ID, VariableKey: "DATABASE_URL"}},
		}},
	}
	if _, err := store.CreateDatabaseService(ctx, input); !errors.Is(err, repository.ErrDatabaseBindingConflict) {
		t.Fatalf("environment-variable replacement did not require explicit confirmation: %v", err)
	}
	input.Instances[0].Bindings[0].ReplaceExisting = true
	created, err := store.CreateDatabaseService(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDatabaseInstanceState(ctx, created.Instances[0].ID, repository.UpdateDatabaseInstanceStateInput{
		Status: "healthy", DesiredState: "running",
	}); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	values, err := buildDockerEnvFromEnvironment(ctx, log, store, app.ID, environments[0].ID, consumer.ID, sealer)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || !strings.HasPrefix(values[0], "DATABASE_URL=postgresql://payments_user:") {
		t.Fatalf("unexpected managed environment: %v", values)
	}
	if !strings.Contains(values[0], "p%40ss%3A%2Fword@postgres-production:5432/payments") {
		t.Fatalf("connection password was not safely encoded: %s", values[0])
	}
}
