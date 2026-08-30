package repository

import (
	"context"
	"testing"

	"github.com/furious-fury/HostForge/internal/models"
)

func TestServiceDeploymentLifecycle(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "TaxIO", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/taxio.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}

	deployment, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: envs[0].ID, TriggerKind: "manual", Actor: "operator", Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Status != models.DeploymentQueued || deployment.ServiceID != service.ID || deployment.EnvironmentID != envs[0].ID {
		t.Fatalf("unexpected deployment: %+v", deployment)
	}
	cancelled, err := store.CancelDeployment(ctx, deployment.ID)
	if err != nil || !cancelled {
		t.Fatalf("cancelled=%v err=%v", cancelled, err)
	}
	stored, err := store.GetServiceDeployment(ctx, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.DeploymentCancelled || stored.CancelledAt == "" {
		t.Fatalf("unexpected cancelled deployment: %+v", stored)
	}
	if stored.Branch != "main" {
		t.Fatalf("deployment branch was not persisted: %+v", stored)
	}
	if _, err := store.UpdateServiceEnvironment(ctx, service.ID, envs[0].ID, "release", false); err != nil {
		t.Fatal(err)
	}
	filtered, _, err := store.ListServiceDeploymentsFiltered(ctx, ServiceDeploymentFilter{Branch: "main", Limit: 10})
	if err != nil || len(filtered) != 1 || filtered[0].ID != deployment.ID {
		t.Fatalf("historical branch filter changed with binding: deployments=%+v err=%v", filtered, err)
	}
	cancelled, err = store.CancelDeployment(ctx, deployment.ID)
	if err != nil || cancelled {
		t.Fatalf("second cancellation should be rejected: cancelled=%v err=%v", cancelled, err)
	}

	release, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: envs[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, release.ID, models.DeploymentSuccess, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateServiceDeployment(ctx, service.ID, envs[0].ID, release.ID); err != nil {
		t.Fatal(err)
	}
	binding, err := store.GetServiceEnvironment(ctx, service.ID, envs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.ActiveDeploymentID != release.ID || binding.DesiredState != "running" {
		t.Fatalf("unexpected active binding: %+v", binding)
	}
	events, err := store.ListPlatformEvents(ctx, app.ID, service.ID, "deployment", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 4 {
		t.Fatalf("expected queued, cancelled, queued, and success events, got %+v", events)
	}
	for _, event := range events {
		if event.ApplicationID != app.ID || event.ServiceID != service.ID {
			t.Fatalf("event lost v2 scope: %+v", event)
		}
	}
}

func TestDeploymentCursorDoesNotSkipEqualTimestamps(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	application, err := store.CreateApplication(ctx, "Cursor", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: application.ID, Name: "web", RepoURL: "https://github.com/acme/web.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		if _, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environments[0].ID}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE deployments SET created_at='2026-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	first, cursor, err := store.ListServiceDeploymentsFiltered(ctx, ServiceDeploymentFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	second, final, err := store.ListServiceDeploymentsFiltered(ctx, ServiceDeploymentFilter{Limit: 2, Cursor: cursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || len(second) != 1 || cursor == "" || final != "" {
		t.Fatalf("unexpected pages: first=%d second=%d cursor=%q final=%q", len(first), len(second), cursor, final)
	}
	seen := make(map[string]bool)
	for _, deployment := range append(first, second...) {
		if seen[deployment.ID] {
			t.Fatalf("deployment %s appeared more than once", deployment.ID)
		}
		seen[deployment.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("saw %d deployments, want 3", len(seen))
	}
}
