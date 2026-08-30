package repository

import (
	"context"
	"strings"
	"testing"
)

// UpdateDeploymentRailpackArtifacts/GetDeploymentRailpackArtifacts round-trip
// a multi-KB payload without going through scanServiceDeployment's positional
// scan, which is the whole point of keeping these columns off the shared
// read path (ADR-0002 §15.6/§15.7). This also proves that untouched scan is
// genuinely untouched: GetServiceDeployment on the same row, after the
// write, must still succeed.
func TestDeploymentRailpackArtifactsRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	app, err := store.CreateApplication(ctx, "Railpack", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/railpack.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environments[0].ID})
	if err != nil {
		t.Fatal(err)
	}

	// Multi-KB, to exercise the same size class real plans run at, not just
	// a trivial placeholder.
	planJSON := `{"steps":{"install":{"commands":["npm ci"]},"build":{"commands":["npm run build"]}},` +
		`"deploy":{"startCommand":"npm start"},"caches":{},"secrets":["DATABASE_URL"],` +
		`"padding":"` + strings.Repeat("x", 4000) + `"}`
	infoJSON := `{"railpackVersion":"v0.23.0","detectedProviders":["node"],` +
		`"resolvedPackages":{"node":{"resolvedVersion":"20.11.0"}},"success":true}`

	if err := store.UpdateDeploymentRailpackArtifacts(ctx, deployment.ID, planJSON, infoJSON); err != nil {
		t.Fatalf("write artifacts: %v", err)
	}

	gotPlan, gotInfo, err := store.GetDeploymentRailpackArtifacts(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("read artifacts: %v", err)
	}
	if gotPlan != planJSON {
		t.Errorf("plan JSON round-trip mismatch: got %d bytes, want %d bytes", len(gotPlan), len(planJSON))
	}
	if gotInfo != infoJSON {
		t.Errorf("info JSON round-trip mismatch: got %q, want %q", gotInfo, infoJSON)
	}

	// The positional scan five other places rely on must still work after
	// this row has multi-KB values in unrelated columns.
	reloaded, err := store.GetServiceDeployment(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("GetServiceDeployment after artifact write: %v", err)
	}
	if reloaded.ID != deployment.ID || reloaded.ServiceID != service.ID {
		t.Fatalf("scanServiceDeployment returned wrong row: %+v", reloaded)
	}
}

// A deployment that never went through a Railpack build (Dockerfile path,
// or simply never reached the write) must read back as empty, not error.
func TestDeploymentRailpackArtifactsDefaultEmpty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	app, err := store.CreateApplication(ctx, "Dockerfile App", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "web", RepoURL: "https://github.com/acme/dockerfile.git", InternalPort: 8080})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environments[0].ID})
	if err != nil {
		t.Fatal(err)
	}

	plan, info, err := store.GetDeploymentRailpackArtifacts(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("get artifacts on unwritten row: %v", err)
	}
	if plan != "" || info != "" {
		t.Fatalf("expected empty defaults, got plan=%q info=%q", plan, info)
	}
}
