package repository

import (
	"context"
	"errors"
	"testing"
)

func TestServiceCRUDAndEnvironmentBindings(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "TaxIO", "Tax platform")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil || len(environments) != 2 {
		t.Fatalf("environments=%d err=%v", len(environments), err)
	}

	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/taxio.git", DeployRuntime: "auto", InternalPort: 3000, HealthCheckPath: "/health"})
	if err != nil {
		t.Fatal(err)
	}
	for _, environment := range environments {
		binding, err := store.GetServiceEnvironment(ctx, service.ID, environment.ID)
		if err != nil {
			t.Fatal(err)
		}
		if binding.Branch != "" || binding.AutoDeploy {
			t.Fatalf("unexpected initial binding: %+v", binding)
		}
	}

	if _, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/other.git", InternalPort: 3000}); !errors.Is(err, ErrDuplicateService) {
		t.Fatalf("expected duplicate service, got %v", err)
	}
	updated, err := store.UpdateService(ctx, service.ID, CreateServiceInput{Name: "backend", RepoURL: service.RepoURL, DeployRuntime: "auto", InternalPort: 8080, HealthCheckPath: "/ready"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "backend" || updated.InternalPort != 8080 || updated.HealthCheckPath != "/ready" {
		t.Fatalf("unexpected update: %+v", updated)
	}
	binding, err := store.UpdateServiceEnvironment(ctx, service.ID, environments[0].ID, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Branch != "main" || !binding.AutoDeploy {
		t.Fatalf("unexpected binding update: %+v", binding)
	}
	if err := store.DeleteService(ctx, service.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetService(ctx, service.ID); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("expected deleted service, got %v", err)
	}
}
