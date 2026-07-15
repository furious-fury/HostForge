package repository

import (
	"context"
	"testing"

	"github.com/hostforge/hostforge/internal/models"
)

func TestAutoDeployTargetsAndRollbackAudit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Release Lab", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/release-lab.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateServiceEnvironment(ctx, service.ID, envs[0].ID, "main", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateServiceEnvironment(ctx, service.ID, envs[1].ID, "staging", true); err != nil {
		t.Fatal(err)
	}

	targets, err := store.ListAutoDeployTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected two independently targeted environments, got %+v", targets)
	}
	if targets[0].ServiceID != service.ID || targets[0].EnvironmentID == targets[1].EnvironmentID {
		t.Fatalf("unexpected targets: %+v", targets)
	}

	source, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: envs[0].ID, CommitHash: "abc123", TriggerKind: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, source.ID, models.DeploymentSuccess, ""); err != nil {
		t.Fatal(err)
	}
	rollback, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: envs[0].ID, CommitHash: source.CommitHash, TriggerKind: "rollback", Actor: "operator", RollbackOf: source.ID})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetServiceDeployment(ctx, rollback.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RollbackOf != source.ID || stored.CommitHash != source.CommitHash || stored.TriggerKind != "rollback" {
		t.Fatalf("rollback audit metadata was not retained: %+v", stored)
	}
}
