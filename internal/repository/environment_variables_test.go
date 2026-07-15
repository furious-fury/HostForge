package repository

import (
	"context"
	"testing"
)

func TestEnvironmentVariableScopesAndSecretMetadata(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Secrets", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := store.UpsertEnvironmentVariable(ctx, app.ID, envs[0].ID, "", "DATABASE_URL", []byte("cipher-one"), "prod")
	if err != nil {
		t.Fatal(err)
	}
	override, err := store.UpsertEnvironmentVariable(ctx, app.ID, envs[0].ID, service.ID, "DATABASE_URL", []byte("cipher-two"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if shared.ServiceID != "" || override.ServiceID != service.ID {
		t.Fatalf("unexpected scopes: shared=%+v override=%+v", shared, override)
	}
	sealed, err := store.ListEnvironmentVariablesSealed(ctx, app.ID, envs[0].ID, service.ID)
	if err != nil || len(sealed) != 1 || string(sealed[0].ValueCT) != "cipher-two" {
		t.Fatalf("service override did not win: rows=%+v err=%v", sealed, err)
	}
	meta, err := store.ListEnvironmentVariableMeta(ctx, app.ID, envs[0].ID, "")
	if err != nil || len(meta) != 1 || meta[0].ValueLast4 != "prod" {
		t.Fatalf("unexpected shared metadata: rows=%+v err=%v", meta, err)
	}
	updated, err := store.UpdateEnvironmentVariableValue(ctx, app.ID, envs[0].ID, shared.ID, []byte("cipher-new"), "last")
	if err != nil || updated.ValueLast4 != "last" {
		t.Fatalf("unexpected update: item=%+v err=%v", updated, err)
	}
	if err := store.DeleteEnvironmentVariable(ctx, app.ID, envs[0].ID, override.ID); err != nil {
		t.Fatal(err)
	}
}
