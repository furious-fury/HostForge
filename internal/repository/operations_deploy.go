package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/furious-fury/HostForge/internal/models"
)

// This file is the deploy-specific half of the operations queue projection
// (ADR-0002 §4, §5, phase 2). deployments and operations share a primary
// key — operations.id == deployments.id — the same trick Phase 1 used for
// database_operations. operations is authoritative for queueing; deployments
// stays the read model every existing endpoint and screen already uses.
//
// Every write here is guarded on deployments' CURRENT status, which is what
// makes the whole scheme safe without ever checking an operation's kind: an
// id belongs to at most one domain table, so "id=? AND
// deployments.status=<expected>" can never touch the wrong domain's row
// (deployments has none for a database-operation id, so it matches zero
// rows and no-ops), and can never reopen a deployment the handler — or an
// operator's cancel — already made terminal.

// DeployOperationKind is the operations.kind every deploy is queued under.
const DeployOperationKind = "deploy"

// EnqueueDeployOperationInput describes one deploy's operations-queue row.
// LockKey is taken as given rather than computed from ServiceID and
// EnvironmentID here: services.DeployLockKey is the one function that formats
// it, also used as the git worktree scope, and repository cannot import
// services without a cycle.
type EnqueueDeployOperationInput struct {
	DeploymentID  string
	LockKey       string
	ApplicationID string
	ServiceID     string
	EnvironmentID string
	Actor         string
	Priority      int
}

// EnqueueDeployOperation queues a deploy for the deploy runtime to claim.
// DeploymentID is reused as the operation id -- the shared-primary-key
// projection this file implements depends on the two matching exactly.
// MaxAttempts is fixed at 1: an interrupted or exhausted deploy must fail,
// never silently resume (ADR-0002 §4.3, §10).
func (s *Store) EnqueueDeployOperation(ctx context.Context, in EnqueueDeployOperationInput) (Operation, error) {
	return s.EnqueueOperation(ctx, NewOperationInput{
		ID:            in.DeploymentID,
		Kind:          DeployOperationKind,
		LockKey:       in.LockKey,
		MaxAttempts:   1,
		Priority:      in.Priority,
		ApplicationID: in.ApplicationID,
		ServiceID:     in.ServiceID,
		EnvironmentID: in.EnvironmentID,
		Actor:         in.Actor,
	})
}

// recordDeploymentStatusEventTx writes the platform_events row for a status
// transition. Shared by UpdateDeploymentStatus and every projection below,
// so the events timeline can never diverge from the status column — one
// write path, however many callers.
func recordDeploymentStatusEventTx(ctx context.Context, tx *sql.Tx, deploymentID, status, detail, now string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO platform_events(application_id,service_id,environment_id,deployment_id,event_type,status,actor,message,detail,created_at)
		SELECT COALESCE(svc.application_id,''),d.service_id,d.environment_id,d.id,
		       'deployment',?,COALESCE(d.actor,''),'Deployment ' || lower(?),?,?
		FROM deployments d JOIN services svc ON svc.id=d.service_id WHERE d.id=?`,
		status, status, detail, now, deploymentID)
	if err != nil {
		return fmt.Errorf("record deployment status event: %w", err)
	}
	return nil
}

// projectDeployClaimTx marks a deployment BUILDING when its operation is
// claimed. No-op if id is not a deployment, or the deployment is not
// QUEUED — an operator's cancel can race a claim, and the cancel wins.
func projectDeployClaimTx(ctx context.Context, tx *sql.Tx, deploymentID, now string) error {
	result, err := tx.ExecContext(ctx,
		`UPDATE deployments SET status=?,updated_at=? WHERE id=? AND status=?`,
		models.DeploymentBuilding, now, deploymentID, models.DeploymentQueued)
	if err != nil {
		return fmt.Errorf("project deploy claim: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return nil
	}
	return recordDeploymentStatusEventTx(ctx, tx, deploymentID, models.DeploymentBuilding, "", now)
}

// projectDeployDeferTx returns a deployment to QUEUED when its operation is
// deferred.
//
// Nothing takes this path today — ExecuteDeploy's handler has no
// ReadinessChecker, so a deploy is never deferred. Implemented anyway:
// leaving a hole in the mapping is how the next person who adds one
// reintroduces a hang, and BUILDING→QUEUED is already a status the UI
// treats identically to QUEUED (every gate in deployment-screens.tsx checks
// both together).
func projectDeployDeferTx(ctx context.Context, tx *sql.Tx, deploymentID, now string) error {
	result, err := tx.ExecContext(ctx,
		`UPDATE deployments SET status=?,updated_at=? WHERE id=? AND status=?`,
		models.DeploymentQueued, now, deploymentID, models.DeploymentBuilding)
	if err != nil {
		return fmt.Errorf("project deploy defer: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return nil
	}
	return recordDeploymentStatusEventTx(ctx, tx, deploymentID, models.DeploymentQueued, "", now)
}

// projectDeployCompleteTx records a terminal transition, translating from
// the queue's status vocabulary (success/failed/cancelled) to deployments'
// (SUCCESS/FAILED/CANCELLED) so no caller has to.
//
// This is a no-op for the ordinary path: ExecuteDeploy already writes
// SUCCESS or FAILED before its handler returns, and CancelDeployment
// already writes CANCELLED synchronously from the HTTP handler — in both
// cases the guard on non-terminal status matches nothing here. It exists
// for the transitions the deploy handler cannot make itself: no handler
// registered for the kind, a readiness failure, or boot recovery — so a
// deployment can never be left non-terminal with no operation left to
// finish it.
func projectDeployCompleteTx(ctx context.Context, tx *sql.Tx, deploymentID, status, errorMessage, now string) error {
	var deployStatus string
	switch status {
	case "success":
		deployStatus = models.DeploymentSuccess
	case "failed":
		deployStatus = models.DeploymentFailed
	case "cancelled":
		deployStatus = models.DeploymentCancelled
	default:
		return fmt.Errorf("project deploy complete: invalid status %q", status)
	}

	var result sql.Result
	var err error
	if deployStatus == models.DeploymentCancelled {
		result, err = tx.ExecContext(ctx,
			`UPDATE deployments SET status=?,cancelled_at=?,updated_at=? WHERE id=? AND status IN (?,?)`,
			deployStatus, now, now, deploymentID, models.DeploymentQueued, models.DeploymentBuilding)
	} else {
		result, err = tx.ExecContext(ctx,
			`UPDATE deployments SET status=?,error_message=?,updated_at=? WHERE id=? AND status IN (?,?)`,
			deployStatus, errorMessage, now, deploymentID, models.DeploymentQueued, models.DeploymentBuilding)
	}
	if err != nil {
		return fmt.Errorf("project deploy complete: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return nil
	}
	return recordDeploymentStatusEventTx(ctx, tx, deploymentID, deployStatus, errorMessage, now)
}

// projectExhaustedDeploysTx fails, as interrupted, every deployment backing
// an operation RecoverOperations is about to fail as exhausted.
//
// operationIDsSubquery must be the exact "SELECT id FROM operations WHERE
// ..." text RecoverOperations already built for its own operations-side
// UPDATE, reused verbatim here so the two can never select a different set
// of rows. It runs before that UPDATE changes operations' status, while the
// subquery — which only reads operations, never deployments — still
// selects the intended set.
func projectExhaustedDeploysTx(ctx context.Context, tx *sql.Tx, operationIDsSubquery, now string) (int64, error) {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO platform_events(application_id,service_id,environment_id,deployment_id,event_type,status,actor,message,detail,created_at)
		SELECT COALESCE(svc.application_id,''),d.service_id,d.environment_id,d.id,
		       'deployment',?,COALESCE(d.actor,''),'Deployment ' || lower(?),?,?
		FROM deployments d JOIN services svc ON svc.id=d.service_id
		WHERE d.status IN (?,?) AND d.id IN (`+operationIDsSubquery+`)`,
		models.DeploymentFailed, models.DeploymentFailed, "interrupted", now,
		models.DeploymentQueued, models.DeploymentBuilding); err != nil {
		return 0, fmt.Errorf("record exhausted deploy events: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE deployments SET status=?,error_message=?,updated_at=?
		WHERE status IN (?,?) AND id IN (`+operationIDsSubquery+`)`,
		models.DeploymentFailed, "interrupted", now, models.DeploymentQueued, models.DeploymentBuilding)
	if err != nil {
		return 0, fmt.Errorf("project exhausted deploys: %w", err)
	}
	count, _ := result.RowsAffected()
	return count, nil
}

// sweepOrphanedDeploymentsTx fails, as interrupted, every deployment still
// QUEUED or BUILDING with no live operation left to finish it — ADR-0002
// §4.3's "today nothing does this", plus the real backlog of deployments
// stuck by a pre-Phase-2 crash on any existing install (nothing recovered
// those; there was nothing to recover them with).
//
// Must run after RecoverOperations has finished settling operations —
// requeued and failed both — so a legitimately recovered (requeued)
// operation's deployment is never swept out from under it.
func sweepOrphanedDeploymentsTx(ctx context.Context, tx *sql.Tx, now string) (int64, error) {
	const liveOperations = `SELECT id FROM operations WHERE status IN ('queued','running')`
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO platform_events(application_id,service_id,environment_id,deployment_id,event_type,status,actor,message,detail,created_at)
		SELECT COALESCE(svc.application_id,''),d.service_id,d.environment_id,d.id,
		       'deployment',?,COALESCE(d.actor,''),'Deployment ' || lower(?),?,?
		FROM deployments d JOIN services svc ON svc.id=d.service_id
		WHERE d.status IN (?,?) AND d.id NOT IN (`+liveOperations+`)`,
		models.DeploymentFailed, models.DeploymentFailed, "interrupted", now,
		models.DeploymentQueued, models.DeploymentBuilding); err != nil {
		return 0, fmt.Errorf("record orphaned deploy events: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE deployments SET status=?,error_message=?,updated_at=?
		WHERE status IN (?,?) AND id NOT IN (`+liveOperations+`)`,
		models.DeploymentFailed, "interrupted", now, models.DeploymentQueued, models.DeploymentBuilding)
	if err != nil {
		return 0, fmt.Errorf("sweep orphaned deployments: %w", err)
	}
	count, _ := result.RowsAffected()
	return count, nil
}
