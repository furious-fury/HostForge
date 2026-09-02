package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/furious-fury/HostForge/internal/workers"
)

func TestDeployPriorityMapsWebhookBelowOperatorTriggers(t *testing.T) {
	cases := map[string]int{
		"manual":      manualDeployPriority,
		"redeploy":    manualDeployPriority,
		"rollback":    manualDeployPriority,
		"github_push": webhookDeployPriority,
	}
	for trigger, want := range cases {
		if got := deployPriority(trigger); got != want {
			t.Errorf("deployPriority(%q) = %d, want %d", trigger, got, want)
		}
	}
	if manualDeployPriority <= webhookDeployPriority {
		t.Fatalf("manual priority (%d) must outrank webhook priority (%d): a webhook fanning out to several deploys must not starve one operator waiting on a single result",
			manualDeployPriority, webhookDeployPriority)
	}
}

func TestDeployContainerNameFromImageRefRoundTripsWithPrepare(t *testing.T) {
	name, err := deployContainerNameFromImageRef("hostforge/0123456789abcdef0123456789abcdef:20260101t000000-aabbccdd")
	if err != nil {
		t.Fatal(err)
	}
	want := "hostforge-0123456789ab-20260101t000000-aabbccdd"
	if name != want {
		t.Fatalf("container name = %q, want %q", name, want)
	}
}

func TestDeployContainerNameFromImageRefRejectsMalformedRefs(t *testing.T) {
	cases := []string{
		"no-colon-at-all",
		"hostforge/tooshort:buildid",
		"hostforge/0123456789abcdef:",
	}
	for _, imageRef := range cases {
		if _, err := deployContainerNameFromImageRef(imageRef); err == nil {
			t.Errorf("deployContainerNameFromImageRef(%q): expected an error, got none", imageRef)
		}
	}
}

// buildTestDeployTarget creates an application, service, and environment
// binding sufficient for PrepareServiceDeploy and LoadDeployJob, both of
// which need a real store since they write and read deployment/operation
// rows.
func buildTestDeployTarget(t *testing.T, alias string) (*config.Config, *repository.Store, DeployTarget) {
	t.Helper()
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, filepath.Join(t.TempDir(), "hostforge.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)

	app, err := store.CreateApplication(ctx, "Queue "+alias, "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("list environments: %v", err)
	}
	svc, err := store.CreateService(ctx, repository.CreateServiceInput{
		ApplicationID: app.ID, Name: alias, RepoURL: "https://github.com/acme/" + alias + ".git",
		InternalPort: 3000, HealthCheckPath: "/",
		InitialEnvironmentID: environments[0].ID, InitialBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{DataDir: t.TempDir()}
	target, err := ResolveDeployTarget(ctx, store, svc.ID, environments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, store, target
}

// LoadDeployJob must reconstruct the same job PrepareServiceDeploy produced
// -- it is the handler's only way to get one, since ContainerName is never
// persisted and Worktree is read back from the row rather than recomputed.
func TestLoadDeployJobRoundTripsWithPrepareServiceDeploy(t *testing.T) {
	ctx := context.Background()
	cfg, store, target := buildTestDeployTarget(t, "roundtrip")

	prepared, err := PrepareServiceDeploy(ctx, cfg, store, target, "manual", "operator", "", "")
	if err != nil {
		t.Fatal(err)
	}

	deployment, err := store.GetServiceDeployment(ctx, prepared.Deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadDeployJob(ctx, cfg, store, deployment)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.RepoURL != prepared.RepoURL {
		t.Errorf("RepoURL = %q, want %q", loaded.RepoURL, prepared.RepoURL)
	}
	if loaded.Branch != prepared.Branch {
		t.Errorf("Branch = %q, want %q", loaded.Branch, prepared.Branch)
	}
	if loaded.Worktree != prepared.Worktree {
		t.Errorf("Worktree = %q, want %q", loaded.Worktree, prepared.Worktree)
	}
	if loaded.BuildDirectory != prepared.BuildDirectory {
		t.Errorf("BuildDirectory = %q, want %q", loaded.BuildDirectory, prepared.BuildDirectory)
	}
	if loaded.ImageRef != prepared.ImageRef {
		t.Errorf("ImageRef = %q, want %q", loaded.ImageRef, prepared.ImageRef)
	}
	if loaded.ContainerName != prepared.ContainerName {
		t.Errorf("ContainerName = %q, want %q", loaded.ContainerName, prepared.ContainerName)
	}
	if loaded.LogsPath != prepared.LogsPath {
		t.Errorf("LogsPath = %q, want %q", loaded.LogsPath, prepared.LogsPath)
	}
	if loaded.Deployment.ID != prepared.Deployment.ID {
		t.Errorf("Deployment.ID = %q, want %q", loaded.Deployment.ID, prepared.Deployment.ID)
	}
}

// PrepareServiceDeploy is the sole writer of a deploy's operations row. This
// pins the mapping onto the queue: kind, lock_key, and MaxAttempts=1 are
// what let the claim-side kind filter and the drain-timeout fix (commits 1
// and 2 of this phase) do anything at all for a deploy.
func TestPrepareServiceDeployEnqueuesAClaimableOperation(t *testing.T) {
	ctx := context.Background()
	cfg, store, target := buildTestDeployTarget(t, "enqueue")

	job, err := PrepareServiceDeploy(ctx, cfg, store, target, "github_push", "github", "", "")
	if err != nil {
		t.Fatal(err)
	}

	op, err := store.GetOperation(ctx, job.Deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if op.Kind != repository.DeployOperationKind {
		t.Errorf("kind = %q, want %q", op.Kind, repository.DeployOperationKind)
	}
	if want := DeployLockKey(target.Service.ID, target.Environment.ID); op.LockKey != want {
		t.Errorf("lock key = %q, want %q", op.LockKey, want)
	}
	if op.MaxAttempts != 1 {
		t.Errorf("max attempts = %d, want 1 (a deploy must never silently resume)", op.MaxAttempts)
	}
	if op.Priority != webhookDeployPriority {
		t.Errorf("priority = %d, want %d for a github_push trigger", op.Priority, webhookDeployPriority)
	}
	if op.Status != "queued" {
		t.Errorf("status = %q, want queued", op.Status)
	}

	claimed, err := store.ClaimNextOperation(ctx, repository.ClaimOptions{
		Owner: "worker-a", Lease: time.Minute, Kinds: []string{repository.DeployOperationKind},
	})
	if err != nil {
		t.Fatalf("operation was not claimable: %v", err)
	}
	if claimed.ID != job.Deployment.ID {
		t.Fatalf("claimed operation id = %q, want %q", claimed.ID, job.Deployment.ID)
	}
}

func TestDeployOperationHandlerReadyRejectsATerminalDeployment(t *testing.T) {
	ctx := context.Background()
	cfg, store, target := buildTestDeployTarget(t, "ready")
	job, err := PrepareServiceDeploy(ctx, cfg, store, target, "manual", "operator", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// CancelDeployment writes deployments directly and does not touch
	// operations -- exactly the race Ready exists to close (see its doc
	// comment in deploy_operations_handler.go).
	if changed, err := store.CancelDeployment(ctx, job.Deployment.ID); err != nil || !changed {
		t.Fatalf("cancel deployment: changed=%v err=%v", changed, err)
	}

	handler := &deployOperationHandler{log: discardLogger(), cfg: cfg, store: store}
	op, err := store.GetOperation(ctx, job.Deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Ready(ctx, op); err != workers.ErrOperationObsolete {
		t.Fatalf("Ready() error = %v, want workers.ErrOperationObsolete", err)
	}
}

func TestDeployOperationHandlerReadyAllowsAQueuedOrBuildingDeployment(t *testing.T) {
	ctx := context.Background()
	cfg, store, target := buildTestDeployTarget(t, "ready-ok")
	job, err := PrepareServiceDeploy(ctx, cfg, store, target, "manual", "operator", "", "")
	if err != nil {
		t.Fatal(err)
	}
	handler := &deployOperationHandler{log: discardLogger(), cfg: cfg, store: store}
	op, err := store.GetOperation(ctx, job.Deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retryAfter, err := handler.Ready(ctx, op); err != nil || retryAfter != 0 {
		t.Fatalf("Ready() = (%v, %v), want (0, nil) for a freshly queued deployment", retryAfter, err)
	}
}

func TestDeployOperationHandlerReadyTreatsAMissingDeploymentAsObsolete(t *testing.T) {
	ctx := context.Background()
	cfg, store, _ := buildTestDeployTarget(t, "ready-missing")
	handler := &deployOperationHandler{log: discardLogger(), cfg: cfg, store: store}
	op := workers.Operation{ID: "does-not-exist"}
	if _, err := handler.Ready(ctx, op); err != workers.ErrOperationObsolete {
		t.Fatalf("Ready() error = %v, want workers.ErrOperationObsolete", err)
	}
}
