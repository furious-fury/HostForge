package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hostforge/hostforge/internal/database"
	"github.com/hostforge/hostforge/internal/models"
)

func TestProjectDeployConfigRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := database.OpenSQLite(ctx, filepath.Join(dir, "hf.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := New(db)
	p, err := store.CreateProject(ctx, CreateProjectInput{
		Name:             "app",
		RepoURL:          "https://github.com/example/app",
		Branch:           "main",
		DeployRuntime:    models.DeployRuntimeBun,
		DeployInstallCmd: "bun install --frozen-lockfile",
		DeployBuildCmd:   "",
		DeployStartCmd:   "bun run start",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.DeployRuntime != models.DeployRuntimeBun {
		t.Fatalf("runtime=%q", p.DeployRuntime)
	}

	loaded, err := store.GetProjectByID(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DeployInstallCmd != "bun install --frozen-lockfile" || loaded.DeployStartCmd != "bun run start" {
		t.Fatalf("loaded mismatch: %+v", loaded)
	}

	updated, err := store.UpdateProjectDeployConfig(ctx, p.ID, models.DeployRuntimeAuto, "npm ci", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if updated.DeployRuntime != models.DeployRuntimeAuto || updated.DeployInstallCmd != "npm ci" {
		t.Fatalf("updated mismatch: %+v", updated)
	}
}
