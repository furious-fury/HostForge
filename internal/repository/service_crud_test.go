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

func TestApplicationLifecycleAndServiceEnvironmentList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	app, err := store.CreateApplication(ctx, "Ledger", "Initial")
	if err != nil {
		t.Fatal(err)
	}
	archived := true
	updated, err := store.UpdateApplication(ctx, app.ID, "Ledger API", "Updated", &archived)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Ledger API" || updated.Description != "Updated" || !updated.Archived {
		t.Fatalf("unexpected application update: %+v", updated)
	}

	service, err := store.CreateService(ctx, CreateServiceInput{
		ApplicationID:   app.ID,
		Name:            "api",
		RepoURL:         "https://github.com/acme/ledger.git",
		DeployRuntime:   "auto",
		InternalPort:    8080,
		HealthCheckPath: "/health",
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := store.ListServiceEnvironments(ctx, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 {
		t.Fatalf("expected production and staging bindings, got %d", len(bindings))
	}

	if err := store.DeleteApplication(ctx, app.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetService(ctx, service.ID); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("expected cascading service deletion, got %v", err)
	}
	if err := store.DeleteApplication(ctx, app.ID); !errors.Is(err, ErrApplicationNotFound) {
		t.Fatalf("expected missing application error, got %v", err)
	}
}

func TestCreateEnvironmentBindsExistingServicesTransactionally(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	application, err := store.CreateApplication(ctx, "Ledger", "")
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/ledger.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := store.CreateEnvironment(ctx, application.ID, "QA", "qa", "staging")
	if err != nil {
		t.Fatal(err)
	}
	if environment.Name != "QA" || environment.Slug != "qa" || environment.Kind != "staging" {
		t.Fatalf("unexpected environment: %+v", environment)
	}
	if _, err := store.GetServiceEnvironment(ctx, service.ID, environment.ID); err != nil {
		t.Fatalf("new environment missing service binding: %v", err)
	}
	if _, err := store.CreateEnvironment(ctx, application.ID, "Duplicate", "qa", "staging"); !errors.Is(err, ErrDuplicateEnvironment) {
		t.Fatalf("expected duplicate environment, got %v", err)
	}
}

func TestApplicationSummaryIncludesHealthDomainAndLatestDeployment(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	application, err := store.CreateApplication(ctx, "Ledger", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, _ := store.ListApplicationEnvironments(ctx, application.ID)
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/ledger.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	production := environments[0]
	for _, environment := range environments {
		if environment.Kind == "production" {
			production = environment
		}
	}
	deployment, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: production.ID, CommitHash: "abc123"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, deployment.ID, "SUCCESS", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateServiceDeployment(ctx, service.ID, production.ID, deployment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateServiceDomain(ctx, application.ID, production.ID, service.ID, "ledger.example.test"); err != nil {
		t.Fatal(err)
	}
	summaries, err := store.ListApplicationSummaries(ctx)
	if err != nil || len(summaries) != 1 {
		t.Fatalf("summaries=%+v err=%v", summaries, err)
	}
	summary := summaries[0]
	if summary.ServiceCount != 1 || summary.HealthyServiceCount != 1 || summary.DomainCount != 1 || summary.LatestDeployment == nil || summary.LatestDeployment.ID != deployment.ID {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}
