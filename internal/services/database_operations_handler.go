package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/furious-fury/HostForge/internal/workers"
	mobyclient "github.com/moby/moby/client"
)

// databaseOperationKinds maps each claimable database operation type to its
// queue kind.
//
// 'delete' and 'purge' are absent deliberately. FinalizeDatabaseServiceDeletion
// writes a terminal 'delete' row as an audit record, so it is never queued
// and never claimed; 'purge' has no writer at all. See the note above the
// dispatch switch in processDatabaseOperation.
var databaseOperationKinds = []string{
	"provision",
	"restore_deleted",
	"start",
	"stop",
	"restart",
	"backup",
	"restore",
	"rotate_credentials",
	"upgrade",
}

// DatabaseOperationKind returns the queue kind for a database operation type.
func DatabaseOperationKind(operationType string) string {
	return "db_" + strings.TrimSpace(operationType)
}

// databaseOperationHandler runs one kind of database operation.
//
// It is a thin adapter: the work itself stays in processDatabaseOperation and
// the per-type functions it dispatches to, unchanged. Those already record
// their own progress and terminal status through UpdateDatabaseOperation,
// which writes both the queue row and its projection, so the runtime's own
// completion call finds nothing to update and no-ops.
type databaseOperationHandler struct {
	kind             string
	log              *slog.Logger
	store            *repository.Store
	sealer           *envcrypt.Sealer
	dockerClient     *mobyclient.Client
	dataDir          string
	minFreeDiskBytes int64
	cfg              *config.Config
}

// NewDatabaseOperationHandlers builds the handler set to register with the
// operations runtime.
func NewDatabaseOperationHandlers(log *slog.Logger, store *repository.Store, sealer *envcrypt.Sealer,
	dockerClient *mobyclient.Client, dataDir string, minFreeDiskBytes int64, cfg *config.Config) []workers.Handler {
	handlers := make([]workers.Handler, 0, len(databaseOperationKinds))
	for _, operationType := range databaseOperationKinds {
		handlers = append(handlers, &databaseOperationHandler{
			kind:             DatabaseOperationKind(operationType),
			log:              log,
			store:            store,
			sealer:           sealer,
			dockerClient:     dockerClient,
			dataDir:          dataDir,
			minFreeDiskBytes: minFreeDiskBytes,
			cfg:              cfg,
		})
	}
	return handlers
}

func (h *databaseOperationHandler) Kind() string { return h.kind }

func (h *databaseOperationHandler) Handle(ctx context.Context, op workers.Operation) error {
	operation, err := h.store.GetDatabaseOperation(ctx, op.ID)
	if err != nil {
		return fmt.Errorf("read database operation: %w", err)
	}

	processDatabaseOperation(ctx, h.log, h.store, h.sealer, h.dockerClient, operation,
		h.dataDir, h.minFreeDiskBytes, h.cfg)

	// processDatabaseOperation reports its own outcome, so by now the row
	// should be terminal. Re-read it: if it is still running, the operation
	// finished without recording anything and reporting success would be a
	// lie. Cancellation is the one legitimate way that happens.
	completed, err := h.store.GetDatabaseOperation(context.WithoutCancel(ctx), op.ID)
	if err != nil {
		return fmt.Errorf("read database operation outcome: %w", err)
	}
	switch completed.Status {
	case "success", "cancelled":
		return nil
	case "failed":
		// Surface the handler's own code rather than a generic one; the row
		// already carries the detail.
		return &workers.HandlerError{
			Code: completed.ErrorCode,
			Err:  errors.New(completed.ErrorMessage),
		}
	default:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return workers.Failf("database_operation_incomplete",
			"operation finished without recording an outcome (status %q)", completed.Status)
	}
}

// Ready re-expresses the two domain gates that used to sit inside the claim
// query as SQL. Keeping them out of the claim is what lets the queue stay
// generic; keeping them here is what keeps them enforced.
func (h *databaseOperationHandler) Ready(ctx context.Context, op workers.Operation) (time.Duration, error) {
	operation, err := h.store.GetDatabaseOperation(ctx, op.ID)
	if err != nil {
		return 0, fmt.Errorf("read database operation: %w", err)
	}

	// Gate one: an instance on its way out makes its pending work obsolete.
	// The claim query used to skip these, which left them queued forever and
	// polled by the UI indefinitely; cancelling them is the fix.
	if operation.DatabaseInstanceID != "" {
		instance, err := h.store.GetDatabaseInstance(ctx, operation.DatabaseInstanceID)
		if err != nil {
			return 0, fmt.Errorf("read database instance: %w", err)
		}
		if instance.DesiredState == "deleted" {
			return 0, workers.ErrOperationObsolete
		}
	}

	// Gate two: a restore waits for a healthy target and for its safety
	// backup to finish. lock_key already orders the safety backup ahead of
	// the restore, so this mostly catches the case where that backup failed.
	if operation.OperationType == "restore" {
		return h.restoreReadiness(ctx, operation)
	}
	return 0, nil
}

// databaseRestoreRetryAfter is how long a restore waits before re-checking
// its target. Longer than the 2-second claim poll on purpose: nothing about
// a restore's preconditions changes that fast.
const databaseRestoreRetryAfter = 5 * time.Second

func (h *databaseOperationHandler) restoreReadiness(ctx context.Context, operation repository.DatabaseOperation) (time.Duration, error) {
	if operation.DatabaseInstanceID == "" {
		return 0, nil
	}
	instance, err := h.store.GetDatabaseInstance(ctx, operation.DatabaseInstanceID)
	if err != nil {
		return 0, fmt.Errorf("read restore target: %w", err)
	}
	if instance.Status != "healthy" || instance.DesiredState != "running" {
		return databaseRestoreRetryAfter, nil
	}

	job, err := h.store.GetDatabaseRestoreJob(ctx, operation.ID)
	if err != nil {
		return 0, fmt.Errorf("read restore job: %w", err)
	}
	if strings.TrimSpace(job.SafetyBackupID) == "" {
		return 0, nil
	}
	safety, err := h.store.GetDatabaseBackup(ctx, job.SafetyBackupID)
	if err != nil {
		return 0, fmt.Errorf("read safety backup: %w", err)
	}
	switch safety.Status {
	case "success", "failed", "cancelled":
		return 0, nil
	default:
		return databaseRestoreRetryAfter, nil
	}
}
