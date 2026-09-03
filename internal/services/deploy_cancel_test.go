package services

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/dockertest"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/obs"
	"github.com/furious-fury/HostForge/internal/repository"
)

// ExecuteDeploy used to have zero step-boundary cancellation checks
// (ADR-0002 §4.4 claims otherwise; it does not hold). stepBoundary is the
// helper every boundary calls; these tests pin its two behaviors directly,
// since ExecuteDeploy itself has no seam to observe a boundary firing
// mid-run without a live Docker daemon.

func TestStepBoundaryIsANoOpOnALiveContext(t *testing.T) {
	job := DeployJob{
		Deployment: models.Deployment{ID: "dep-1"},
		Target: &DeployTarget{
			Service:     repository.Service{ID: "svc-1"},
			Environment: repository.Environment{ID: "env-1"},
		},
	}
	markFailedCalled := false
	cleanupCalled := false
	err := job.stepBoundary(context.Background(), slog.Default(),
		func(error) { markFailedCalled = true },
		func(string) { cleanupCalled = true },
		"health_check", time.Now())
	if err != nil {
		t.Fatalf("expected nil error for a live context, got %v", err)
	}
	if markFailedCalled {
		t.Fatal("markFailed must not run when the context is still live")
	}
	if cleanupCalled {
		t.Fatal("cleanup must not run when the context is still live")
	}
}

func TestStepBoundaryFailsTheDeployOnACancelledContext(t *testing.T) {
	job := DeployJob{
		Deployment: models.Deployment{ID: "dep-1"},
		Target: &DeployTarget{
			Service:     repository.Service{ID: "svc-1"},
			Environment: repository.Environment{ID: "env-1"},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var markFailedErr error
	var cleanupReason string
	err := job.stepBoundary(ctx, slog.Default(),
		func(e error) { markFailedErr = e },
		func(reason string) { cleanupReason = reason },
		"health_check", time.Now())

	if err == nil {
		t.Fatal("expected a non-nil error for a cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(err, context.Canceled) = false, want true (err: %v)", err)
	}
	if markFailedErr == nil {
		t.Fatal("markFailed was not called")
	}
	if !errors.Is(markFailedErr, context.Canceled) {
		t.Fatalf("markFailed was called with %v, want it to unwrap to context.Canceled", markFailedErr)
	}
	if cleanupReason != "cancelled before health_check" {
		t.Fatalf("cleanup reason = %q, want %q", cleanupReason, "cancelled before health_check")
	}
}

// cleanupCandidateContainer used to run on the caller's ctx directly.
// That ctx is cancelled by definition on the cancellation path this exists
// for, so StopAndRemove and the status write failed before ever reaching
// the daemon or the database, leaving a running container behind with a
// RUNNING row nothing ever cleaned up. This proves the detached-context fix
// actually reaches the daemon and updates the row even when ctx is already
// cancelled at the call.
func TestCleanupCandidateContainerRunsOnAnAlreadyCancelledContext(t *testing.T) {
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, filepath.Join(t.TempDir(), "hostforge.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)

	app, err := store.CreateApplication(ctx, "Cleanup", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("environments: %v", err)
	}
	svc, err := store.CreateService(ctx, repository.CreateServiceInput{
		ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/api.git",
		InternalPort: 3000, HealthCheckPath: "/",
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{
		ServiceID: svc.ID, EnvironmentID: environments[0].ID, ImageRef: "img", Worktree: "wt", Branch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	container, err := store.AttachContainer(ctx, repository.AttachContainerInput{
		DeploymentID: deployment.ID, DockerContainerID: "candidate-container", InternalPort: 3000, HostPort: 40000, Status: "RUNNING",
	})
	if err != nil {
		t.Fatal(err)
	}

	var sawStop, sawRemove bool
	dockerClient := dockertest.NewClient(t, func(request *http.Request) (*http.Response, error) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/stop"):
			sawStop = true
			return dockertest.Response(request, http.StatusNoContent, ""), nil
		case request.Method == http.MethodDelete:
			sawRemove = true
			return dockertest.Response(request, http.StatusNoContent, ""), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
			return nil, nil
		}
	})

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	cleanupCandidateContainer(cancelledCtx, slog.Default(), dockerClient, store, "candidate-container", container.ID, "test cancellation")

	if !sawStop {
		t.Error("expected a stop request to reach the daemon despite the cancelled context")
	}
	if !sawRemove {
		t.Error("expected a remove request to reach the daemon despite the cancelled context")
	}
	got, err := store.GetContainerByDeploymentID(ctx, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "REMOVED" {
		t.Fatalf("container status = %q, want REMOVED", got.Status)
	}
}

// The step boundary is only useful if the record of it survives. Every
// observability write inherited the deploy's own context, so cancelling a
// deploy also cancelled the write describing where it stopped: on a real
// host this showed up as "insert deploy_step: context canceled" for both
// health_check and deploy_total, leaving a cancelled deploy with no trace of
// how far it got. This drives the real store through the real SQLite driver
// -- the one that rejected those inserts -- rather than a fake that would
// happily accept a cancelled context.
func TestStepBoundaryRecordsTheCancellationDespiteTheCancelledContext(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostforge.sqlite")
	db, err := database.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)

	job := DeployJob{
		Deployment: models.Deployment{ID: "dep-obs-1"},
		Target: &DeployTarget{
			Service:     repository.Service{ID: "svc-1"},
			Environment: repository.Environment{ID: "env-1"},
		},
	}

	ctx, cancel := context.WithCancel(obs.WithStore(context.Background(), store))
	cancel()

	if err := job.stepBoundary(ctx, slog.Default(), func(error) {}, nil, "health_check", time.Now()); err == nil {
		t.Fatal("stepBoundary returned nil on a cancelled context")
	}

	rows, err := store.ListDeployStepsByDeployment(context.Background(), "dep-obs-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("deploy_steps rows = %d, want 1: the cancellation record was discarded along with the deploy", len(rows))
	}
	if rows[0].Step != "health_check" || rows[0].Status != "cancelled" {
		t.Fatalf("recorded step = %q/%q, want health_check/cancelled", rows[0].Step, rows[0].Status)
	}
}
