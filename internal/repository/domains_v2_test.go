package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/hostforge/hostforge/internal/models"
)

func TestEnsurePlatformServiceDomainIsStableAndCustomDomainsRemainPreferred(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.MarkGitHubAppComplete(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteOnboarding(ctx, "forge.example.com"); err != nil {
		t.Fatal(err)
	}
	app, err := store.CreateApplication(ctx, "Share URLs", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "web", RepoURL: "https://github.com/acme/web.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	generated, created, err := store.EnsurePlatformServiceDomain(ctx, app.ID, environments[0].ID, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !created || generated.Kind != "platform" || !strings.HasSuffix(generated.DomainName, ".forge.example.com") {
		t.Fatalf("unexpected generated domain: created=%v domain=%+v", created, generated)
	}
	reused, created, err := store.EnsurePlatformServiceDomain(ctx, app.ID, environments[0].ID, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	if created || reused.ID != generated.ID || reused.DomainName != generated.DomainName {
		t.Fatalf("platform domain was not stable: first=%+v second=%+v", generated, reused)
	}
	custom, err := store.CreateServiceDomain(ctx, app.ID, environments[0].ID, service.ID, "app.customer.example")
	if err != nil {
		t.Fatal(err)
	}
	domains, err := store.ListServiceDomains(ctx, app.ID, environments[0].ID, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 2 || domains[0].ID != custom.ID || domains[1].ID != generated.ID {
		t.Fatalf("custom domain should be preferred: %+v", domains)
	}
	if err := store.UpdatePlatformDomain(ctx, "forge.example.com", "host.example.net"); err != nil {
		t.Fatal(err)
	}
	moved, err := store.GetServiceDomain(ctx, app.ID, environments[0].ID, generated.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldLabel := strings.TrimSuffix(generated.DomainName, ".forge.example.com")
	if moved.DomainName != oldLabel+".host.example.net" {
		t.Fatalf("managed label was not preserved: before=%s after=%s", generated.DomainName, moved.DomainName)
	}
	customAfterMove, err := store.GetServiceDomain(ctx, app.ID, environments[0].ID, custom.ID)
	if err != nil {
		t.Fatal(err)
	}
	if customAfterMove.DomainName != custom.DomainName {
		t.Fatalf("custom domain changed during platform move: %+v", customAfterMove)
	}
	if err := store.DeleteServiceDomain(ctx, app.ID, environments[0].ID, moved.ID); err != ErrManagedDomain {
		t.Fatalf("managed domain deletion error=%v", err)
	}
}

func TestServiceDomainsFollowEnvironmentActiveRelease(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Routes", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "web", RepoURL: "https://github.com/acme/web.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	wantPorts := map[string]int{}
	for index, env := range envs {
		deployment, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: env.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.UpdateDeploymentStatus(ctx, deployment.ID, models.DeploymentSuccess, ""); err != nil {
			t.Fatal(err)
		}
		port := 18080 + index
		if _, err := store.AttachContainer(ctx, AttachContainerInput{DeploymentID: deployment.ID, DockerContainerID: "container-" + env.Slug, InternalPort: 3000, HostPort: port}); err != nil {
			t.Fatal(err)
		}
		if err := store.ActivateServiceDeployment(ctx, service.ID, env.ID, deployment.ID); err != nil {
			t.Fatal(err)
		}
		domain, err := store.CreateServiceDomain(ctx, app.ID, env.ID, service.ID, env.Slug+".example.com")
		if err != nil {
			t.Fatal(err)
		}
		wantPorts[domain.DomainName] = port
	}
	routes, err := store.ListDomainRoutes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 {
		t.Fatalf("expected two routes, got %d", len(routes))
	}
	for _, route := range routes {
		if route.HostPort != wantPorts[route.DomainName] {
			t.Fatalf("domain %s used port %d, want %d", route.DomainName, route.HostPort, wantPorts[route.DomainName])
		}
	}
}

func TestUpdateServiceDomainCanRetargetWithinEnvironment(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Retarget", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "web", RepoURL: "https://github.com/acme/web.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 8080})
	if err != nil {
		t.Fatal(err)
	}
	domain, err := store.CreateServiceDomain(ctx, app.ID, environments[0].ID, first.ID, "app.example.com")
	if err != nil {
		t.Fatal(err)
	}

	updated, err := store.UpdateServiceDomain(ctx, app.ID, environments[0].ID, domain.ID, "api.example.com", second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DomainName != "api.example.com" || updated.ServiceID != second.ID {
		t.Fatalf("unexpected retargeted domain: %+v", updated)
	}
}

func TestUpdateServiceDomainRejectsServiceOutsideApplication(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Primary", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "web", RepoURL: "https://github.com/acme/web.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	domain, err := store.CreateServiceDomain(ctx, app.ID, environments[0].ID, service.ID, "app.example.com")
	if err != nil {
		t.Fatal(err)
	}
	otherApp, err := store.CreateApplication(ctx, "Other", "")
	if err != nil {
		t.Fatal(err)
	}
	otherService, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: otherApp.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 8080})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.UpdateServiceDomain(ctx, app.ID, environments[0].ID, domain.ID, domain.DomainName, otherService.ID); err != ErrEnvironmentNotFound {
		t.Fatalf("expected ErrEnvironmentNotFound, got %v", err)
	}
}

func TestRestoreServiceDomainPreservesRecord(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Restore", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "web", RepoURL: "https://github.com/acme/web.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	domain, err := store.CreateServiceDomain(ctx, app.ID, environments[0].ID, service.ID, "restore.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteServiceDomain(ctx, app.ID, environments[0].ID, domain.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RestoreServiceDomain(ctx, domain); err != nil {
		t.Fatal(err)
	}
	restored, err := store.GetServiceDomain(ctx, app.ID, environments[0].ID, domain.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored != domain {
		t.Fatalf("restored domain changed: got=%+v want=%+v", restored, domain)
	}
}
