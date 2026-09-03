package repository

import (
	"context"
	"testing"
)

// ListSweepableDeployContainers selects exactly the residue a killed deploy
// leaves: a still-RUNNING container row whose deployment ended terminal and is
// not the one serving its binding. It must exclude the live container, an
// in-flight (non-terminal) deployment, and an already-removed row.
func TestListSweepableDeployContainersSelectsOnlyOrphans(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	application, err := store.CreateApplication(ctx, "Sweep", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	env := environments[0].ID
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}

	// The live deployment: SUCCESS, active, with a running container. Never
	// sweepable -- removing this is an outage.
	live, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: env})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttachContainer(ctx, AttachContainerInput{DeploymentID: live.ID, DockerContainerID: "docker-live", InternalPort: 3000, HostPort: 18080}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, live.ID, "SUCCESS", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateServiceDeployment(ctx, service.ID, env, live.ID); err != nil {
		t.Fatal(err)
	}

	// The orphan: FAILED, not active, container still RUNNING.
	orphan, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: env})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttachContainer(ctx, AttachContainerInput{DeploymentID: orphan.ID, DockerContainerID: "docker-orphan", InternalPort: 3000, HostPort: 18081}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, orphan.ID, "FAILED", "interrupted"); err != nil {
		t.Fatal(err)
	}

	// An in-flight deployment: not active, container RUNNING, but BUILDING is
	// not terminal, so it is legitimately mid-work and must be left alone.
	building, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: env})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttachContainer(ctx, AttachContainerInput{DeploymentID: building.ID, DockerContainerID: "docker-building", InternalPort: 3000, HostPort: 18082}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, building.ID, "BUILDING", ""); err != nil {
		t.Fatal(err)
	}

	// A prior orphan already cleaned up: FAILED and non-active, but its row is
	// REMOVED, so it must not resurface.
	removed, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: env})
	if err != nil {
		t.Fatal(err)
	}
	removedContainer, err := store.AttachContainer(ctx, AttachContainerInput{DeploymentID: removed.ID, DockerContainerID: "docker-removed", InternalPort: 3000, HostPort: 18083})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, removed.ID, "CANCELLED", "cancelled"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateContainerStatus(ctx, removedContainer.ID, "REMOVED"); err != nil {
		t.Fatal(err)
	}

	candidates, err := store.ListSweepableDeployContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("sweepable containers = %d, want 1 (only the FAILED non-active running orphan): %+v", len(candidates), candidates)
	}
	got := candidates[0]
	if got.DockerContainerID != "docker-orphan" {
		t.Fatalf("selected %q, want docker-orphan", got.DockerContainerID)
	}
	if got.DeploymentID != orphan.ID || got.ServiceID != service.ID || got.EnvironmentID != env {
		t.Fatalf("identity fields wrong: %+v", got)
	}
}

// A CANCELLED non-active deployment is also terminal residue and must be
// swept, so both terminal states are covered rather than just FAILED.
func TestListSweepableDeployContainersIncludesCancelled(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	application, err := store.CreateApplication(ctx, "SweepCancel", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environments[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttachContainer(ctx, AttachContainerInput{DeploymentID: cancelled.ID, DockerContainerID: "docker-cancelled", InternalPort: 3000, HostPort: 18090}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, cancelled.ID, "CANCELLED", "cancelled"); err != nil {
		t.Fatal(err)
	}

	candidates, err := store.ListSweepableDeployContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].DockerContainerID != "docker-cancelled" {
		t.Fatalf("cancelled orphan not selected: %+v", candidates)
	}
}
