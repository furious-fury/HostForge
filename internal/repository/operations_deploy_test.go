package repository

import (
	"context"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/models"
)

// deployFixture is a service + environment + one QUEUED deployment, with its
// paired operations row already enqueued — the state PrepareServiceDeploy
// leaves behind, which is where every projection test starts.
type deployFixture struct {
	store      *Store
	service    Service
	deployment models.Deployment
	lockKey    string
}

func newDeployFixture(t *testing.T, alias string) deployFixture {
	t.Helper()
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Deploys", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{
		ApplicationID: app.ID, Name: alias, RepoURL: "https://github.com/acme/" + alias + ".git", InternalPort: 3000,
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
	if _, err := store.EnqueueOperation(ctx, NewOperationInput{
		ID: deployment.ID, Kind: "deploy", LockKey: lockKey, MaxAttempts: 1,
		ApplicationID: app.ID, ServiceID: service.ID, EnvironmentID: envs[0].ID, Actor: "operator",
	}); err != nil {
		t.Fatal(err)
	}
	return deployFixture{store: store, service: service, deployment: deployment, lockKey: lockKey}
}

func (f deployFixture) deploymentStatus(t *testing.T) models.Deployment {
	t.Helper()
	d, err := f.store.GetServiceDeployment(context.Background(), f.deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f deployFixture) deploymentEventCount(t *testing.T) int {
	t.Helper()
	events, err := f.store.ListPlatformEvents(context.Background(), "", f.service.ID, "deployment", 50)
	if err != nil {
		t.Fatal(err)
	}
	return len(events)
}

// --- claim: QUEUED -> BUILDING ---

func TestClaimProjectsDeployToBuilding(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "claim")
	ctx := context.Background()

	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-a", Lease: time.Minute}); err != nil {
		t.Fatal(err)
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentBuilding {
		t.Fatalf("status = %q, want BUILDING", got.Status)
	}
	if f.deploymentEventCount(t) != 2 { // the initial QUEUED event, plus BUILDING
		t.Fatalf("event count = %d, want 2", f.deploymentEventCount(t))
	}
}

// --- defer: BUILDING -> QUEUED (unreachable in production for deploys, see
// the comment on projectDeployDeferTx, but must not corrupt the row if it
// ever fires) ---

func TestDeferProjectsDeployBackToQueued(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "defer")
	ctx := context.Background()

	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-a", Lease: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DeferOperation(ctx, f.deployment.ID, "worker-a", 0); err != nil {
		t.Fatal(err)
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentQueued {
		t.Fatalf("status = %q, want QUEUED", got.Status)
	}
}

// --- complete: success / failed / cancelled ---

func TestCompleteProjectsDeploySuccess(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "success")
	ctx := context.Background()

	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-a", Lease: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.CompleteOperation(ctx, CompleteOperationInput{ID: f.deployment.ID, Owner: "worker-a", Status: "success"}); err != nil {
		t.Fatal(err)
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentSuccess {
		t.Fatalf("status = %q, want SUCCESS", got.Status)
	}
}

func TestCompleteProjectsDeployFailedWithErrorMessage(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "failed")
	ctx := context.Background()

	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-a", Lease: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.CompleteOperation(ctx, CompleteOperationInput{
		ID: f.deployment.ID, Owner: "worker-a", Status: "failed", ErrorMessage: "build_failed",
	}); err != nil {
		t.Fatal(err)
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentFailed || got.ErrorMessage != "build_failed" {
		t.Fatalf("unexpected deployment: %+v", got)
	}
}

func TestCompleteProjectsDeployCancelledWithTimestamp(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "cancelled")
	ctx := context.Background()

	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-a", Lease: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.CompleteOperation(ctx, CompleteOperationInput{ID: f.deployment.ID, Owner: "worker-a", Status: "cancelled"}); err != nil {
		t.Fatal(err)
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentCancelled || got.CancelledAt == "" {
		t.Fatalf("unexpected deployment: %+v", got)
	}
}

// A deployment ExecuteDeploy already made terminal must never be reopened by
// a later queue transition. This is the invariant the whole design rests on.
func TestCompleteNeverReopensATerminalDeployment(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "noreopen")
	ctx := context.Background()

	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-a", Lease: time.Minute}); err != nil {
		t.Fatal(err)
	}
	// The handler writes SUCCESS directly, as ExecuteDeploy does, before the
	// runtime's own completion call runs.
	if err := f.store.UpdateDeploymentStatus(ctx, f.deployment.ID, models.DeploymentSuccess, ""); err != nil {
		t.Fatal(err)
	}
	before := f.deploymentEventCount(t)

	// The runtime's own completion call, arriving after the handler already
	// finished the row.
	if err := f.store.CompleteOperation(ctx, CompleteOperationInput{ID: f.deployment.ID, Owner: "worker-a", Status: "success"}); err != nil {
		t.Fatal(err)
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentSuccess {
		t.Fatalf("status = %q, want SUCCESS (unchanged)", got.Status)
	}
	if after := f.deploymentEventCount(t); after != before {
		t.Fatalf("event count changed from %d to %d: a no-op transition must not record a duplicate event", before, after)
	}
}

// The same invariant from the recovery side: an operation recovery is about
// to fail as exhausted must not reopen a deployment the handler already
// completed in the same window. This can genuinely race — the handler can
// finish and write SUCCESS an instant before a crash, with the operation
// row itself not yet marked complete when the process dies.
func TestRecoveryDoesNotReopenADeploymentTheHandlerAlreadyFinished(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "norecoverreopen")
	ctx := context.Background()

	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "dead-worker", Lease: time.Hour}); err != nil {
		t.Fatal(err)
	}
	// The handler wrote SUCCESS directly, as ExecuteDeploy does, but the
	// process died before the operation row itself could be completed —
	// it is still "running" and exhausted from the worker's point of view.
	if err := f.store.UpdateDeploymentStatus(ctx, f.deployment.ID, models.DeploymentSuccess, ""); err != nil {
		t.Fatal(err)
	}

	if _, _, err := f.store.RecoverOperations(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentSuccess {
		t.Fatalf("status = %q, want SUCCESS (recovery must not overwrite a completed deployment)", got.Status)
	}
}

// --- recovery ---

func TestRecoveryFailsARunningDeployAsInterrupted(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "recover")
	ctx := context.Background()

	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "dead-worker", Lease: time.Hour}); err != nil {
		t.Fatal(err)
	}
	// max_attempts=1, so the one claim already exhausted it — recovery must
	// fail it, not requeue it, matching "no deploy resume".
	requeued, failed, err := f.store.RecoverOperations(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if failed != 1 || requeued != 0 {
		t.Fatalf("recovered requeued=%d failed=%d, want 0 and 1", requeued, failed)
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentFailed || got.ErrorMessage != "interrupted" {
		t.Fatalf("unexpected deployment after recovery: %+v", got)
	}

	// The operation itself must not be reclaimable afterward.
	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-a", Lease: time.Minute}); err == nil {
		t.Fatal("exhausted deploy operation was claimed again after recovery")
	}
}

// --- orphan sweep ---

// A deployment with no operations row at all — the real backlog on any
// database that crashed before Phase 2 shipped, since nothing recovered
// those — must be failed by the sweep even though it never appears in any
// operations-table subquery.
func TestRecoverySweepsOrphanedDeploymentWithNoOperationRow(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Orphan", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, CreateServiceInput{ApplicationID: app.ID, Name: "orphan", RepoURL: "https://github.com/acme/orphan.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	// CreateServiceDeployment alone, deliberately not paired with an
	// enqueued operation — simulating a pre-Phase-2 row, or one where the
	// enqueue step failed.
	deployment, err := store.CreateServiceDeployment(ctx, CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: envs[0].ID})
	if err != nil {
		t.Fatal(err)
	}

	requeued, failed, err := store.RecoverOperations(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if failed != 1 || requeued != 0 {
		t.Fatalf("recovered requeued=%d failed=%d, want 0 and 1", requeued, failed)
	}
	got, err := store.GetServiceDeployment(ctx, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.DeploymentFailed || got.ErrorMessage != "interrupted" {
		t.Fatalf("orphaned deployment not swept: %+v", got)
	}
}

// A deployment whose operation is legitimately queued or running must never
// be swept, however the sweep is implemented — this is the case that would
// have caught running the sweep before, rather than after, the requeue
// phase settles.
func TestRecoverySweepDoesNotTouchDeploymentsWithLiveOperations(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "live")
	ctx := context.Background()
	// Still queued, never claimed: has a live operation, must be left alone.

	if _, _, err := f.store.RecoverOperations(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentQueued {
		t.Fatalf("status = %q, want QUEUED (untouched)", got.Status)
	}
}

// --- queued cancellation ---

func TestRequestCancellationCancelsAQueuedDeployOutright(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "cancelqueued")
	ctx := context.Background()

	changed, err := f.store.RequestOperationCancellation(ctx, f.deployment.ID)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentCancelled || got.CancelledAt == "" {
		t.Fatalf("queued deployment not cancelled outright: %+v", got)
	}

	// Never claimable afterward.
	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-a", Lease: time.Minute}); err == nil {
		t.Fatal("a cancelled operation was claimed")
	}
}

// A running deploy is asked to stop, not cancelled outright — nothing is
// running to observe an immediate status flip, and the deployment stays
// BUILDING until the handler itself reaches a step boundary.
func TestRequestCancellationOnlyRequestsForARunningDeploy(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "cancelrunning")
	ctx := context.Background()

	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-a", Lease: time.Minute}); err != nil {
		t.Fatal(err)
	}
	changed, err := f.store.RequestOperationCancellation(ctx, f.deployment.ID)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentBuilding {
		t.Fatalf("status = %q, want BUILDING (cancellation is cooperative)", got.Status)
	}

	op, err := f.store.GetOperation(ctx, f.deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if op.CancelRequestedAt.IsZero() {
		t.Fatal("cancel_requested_at not set on the running operation")
	}
}

func TestRequestCancellationOnAlreadyTerminalOperationChangesNothing(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "cancelterminal")
	ctx := context.Background()

	if _, err := f.store.ClaimNextOperation(ctx, ClaimOptions{Owner: "worker-a", Lease: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.CompleteOperation(ctx, CompleteOperationInput{ID: f.deployment.ID, Owner: "worker-a", Status: "success"}); err != nil {
		t.Fatal(err)
	}
	changed, err := f.store.RequestOperationCancellation(ctx, f.deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("cancellation reported a change against an already-terminal operation")
	}
	got := f.deploymentStatus(t)
	if got.Status != models.DeploymentSuccess {
		t.Fatalf("status = %q, want SUCCESS (unchanged)", got.Status)
	}
}
