package repository

import (
	"context"
	"testing"
)

// EnqueueDeployOperation is the one function PrepareServiceDeploy calls to
// queue a deploy. Kind, MaxAttempts, and LockKey are what make the rest of
// this phase's fixes apply to a deploy at all -- the kind filter (commit 1),
// the non-resumable drain path (commit 2), and worktree/lock-key alignment
// (commit 4) all depend on these being set exactly this way.
func TestEnqueueDeployOperationSetsKindLockKeyAndMaxAttempts(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Enqueue", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil || len(envs) == 0 {
		t.Fatalf("list environments: %v", err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{
		ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{
		ServiceID: service.ID, EnvironmentID: envs[0].ID, TriggerKind: "manual", Actor: "operator", Branch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	lockKey := "svc:" + service.ID + ":" + envs[0].ID

	op, err := store.EnqueueDeployOperation(ctx, EnqueueDeployOperationInput{
		DeploymentID: deployment.ID, LockKey: lockKey,
		ApplicationID: app.ID, ServiceID: service.ID, EnvironmentID: envs[0].ID,
		Actor: "operator", Priority: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if op.ID != deployment.ID {
		t.Errorf("operation id = %q, want deployment id %q", op.ID, deployment.ID)
	}
	if op.Kind != DeployOperationKind {
		t.Errorf("kind = %q, want %q", op.Kind, DeployOperationKind)
	}
	if op.LockKey != lockKey {
		t.Errorf("lock key = %q, want %q", op.LockKey, lockKey)
	}
	if op.MaxAttempts != 1 {
		t.Errorf("max attempts = %d, want 1: a deploy must never silently resume", op.MaxAttempts)
	}
	if op.Priority != 200 {
		t.Errorf("priority = %d, want 200", op.Priority)
	}
	if op.Status != "queued" {
		t.Errorf("status = %q, want queued", op.Status)
	}
}
