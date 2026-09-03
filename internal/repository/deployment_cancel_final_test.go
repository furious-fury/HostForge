package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/furious-fury/HostForge/internal/models"
)

// CancelDeployment writes CANCELLED synchronously while the deploy is still
// running, and the deploy only learns it was cancelled at its next step
// boundary. Whatever it concludes in between must not land on the row.
//
// Observed on a real host before this guard: a cancelled deploy ran its
// health check to completion, failed it, and rewrote the row as FAILED --
// leaving cancelled_at set next to an error message. Had the health check
// passed, the same path would have written SUCCESS and put a cancelled
// deploy into production.
func TestCancelledDeploymentCannotBeOverwritten(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "cancelfinal")
	ctx := context.Background()

	if changed, err := f.store.CancelDeployment(ctx, f.deployment.ID); err != nil || !changed {
		t.Fatalf("CancelDeployment: changed=%v err=%v", changed, err)
	}

	for _, status := range []string{models.DeploymentSuccess, models.DeploymentFailed, models.DeploymentBuilding} {
		err := f.store.UpdateDeploymentStatus(ctx, f.deployment.ID, status, "late write")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("UpdateDeploymentStatus(%s) on a cancelled deployment = %v, want sql.ErrNoRows", status, err)
		}
		if got := f.deploymentStatus(t); got.Status != models.DeploymentCancelled {
			t.Fatalf("status after a late %s write = %q, want it to stay CANCELLED", status, got.Status)
		}
	}

	// The error message must stay clean too. A cancelled deployment carrying
	// a failure reason reads as though it failed on its own.
	if got := f.deploymentStatus(t); got.ErrorMessage != "" {
		t.Fatalf("error_message = %q, want empty on a cancelled deployment", got.ErrorMessage)
	}
}

// The guard must not affect a deployment that was never cancelled.
func TestUpdateDeploymentStatusStillWritesUncancelledDeployments(t *testing.T) {
	t.Parallel()
	f := newDeployFixture(t, "uncancelled")
	ctx := context.Background()

	for _, status := range []string{models.DeploymentBuilding, models.DeploymentSuccess} {
		if err := f.store.UpdateDeploymentStatus(ctx, f.deployment.ID, status, ""); err != nil {
			t.Fatalf("UpdateDeploymentStatus(%s): %v", status, err)
		}
		if got := f.deploymentStatus(t); got.Status != status {
			t.Fatalf("status = %q, want %q", got.Status, status)
		}
	}
}
