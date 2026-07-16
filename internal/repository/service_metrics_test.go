package repository

import (
	"context"
	"fmt"
	"testing"
)

func TestServiceMetricSamplesAreScopedOrderedAndBounded(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < serviceMetricRetentionPerBinding+5; i++ {
		_, err := store.InsertServiceMetricSample(ctx, ServiceMetricSample{ServiceID: "svc", EnvironmentID: "prod", CPUPercent: float64(i), SampledAt: fmt.Sprintf("2026-01-01T00:%02d:00Z", i%60)})
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.ListServiceMetricSamples(ctx, "svc", "prod", serviceMetricRetentionPerBinding)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != serviceMetricRetentionPerBinding {
		t.Fatalf("expected bounded retention, got %d", len(rows))
	}
	if rows[0].CPUPercent != 5 || rows[len(rows)-1].CPUPercent != float64(serviceMetricRetentionPerBinding+4) {
		t.Fatalf("samples not returned oldest to newest after trim: first=%v last=%v", rows[0].CPUPercent, rows[len(rows)-1].CPUPercent)
	}
	other, err := store.ListServiceMetricSamples(ctx, "svc", "staging", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("scope leaked: rows=%v err=%v", other, err)
	}
}

func TestActiveServiceMetricTargetsOnlyIncludeRunningBindingsAndContainers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	application, err := store.CreateApplication(ctx, "Metrics", "")
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
	deployment, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environments[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	container, err := store.AttachContainer(ctx, AttachContainerInput{DeploymentID: deployment.ID, DockerContainerID: "docker-active", InternalPort: 3000, HostPort: 18080})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateServiceDeployment(ctx, service.ID, environments[0].ID, deployment.ID); err != nil {
		t.Fatal(err)
	}

	targets, err := store.ListActiveServiceMetricTargets(ctx)
	if err != nil || len(targets) != 1 {
		t.Fatalf("targets=%+v err=%v", targets, err)
	}
	if targets[0].ServiceID != service.ID || targets[0].EnvironmentID != environments[0].ID || targets[0].DockerContainerID != "docker-active" {
		t.Fatalf("unexpected target: %+v", targets[0])
	}

	if err := store.SetServiceDesiredState(ctx, service.ID, environments[0].ID, "stopped"); err != nil {
		t.Fatal(err)
	}
	targets, err = store.ListActiveServiceMetricTargets(ctx)
	if err != nil || len(targets) != 0 {
		t.Fatalf("stopped binding was sampled: targets=%+v err=%v", targets, err)
	}
	if err := store.SetServiceDesiredState(ctx, service.ID, environments[0].ID, "running"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateContainerStatus(ctx, container.ID, "STOPPED"); err != nil {
		t.Fatal(err)
	}
	targets, err = store.ListActiveServiceMetricTargets(ctx)
	if err != nil || len(targets) != 0 {
		t.Fatalf("stopped container was sampled: targets=%+v err=%v", targets, err)
	}
}
