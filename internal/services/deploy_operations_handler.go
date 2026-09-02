package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/obs"
	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/furious-fury/HostForge/internal/reqctx"
	"github.com/furious-fury/HostForge/internal/workers"
	mobyclient "github.com/moby/moby/client"
)

// deployOperationHandler adapts ExecuteDeploy to workers.Handler, run on its
// own workers.Runtime separate from database operations (ADR-0002 phase 2).
// Unlike databaseOperationHandler there is exactly one kind and one domain
// table, so this needs no per-type dispatch.
type deployOperationHandler struct {
	log             *slog.Logger
	cfg             *config.Config
	store           *repository.Store
	sealer          *envcrypt.Sealer
	dockerClient    *mobyclient.Client
	authResolverFor func(context.Context) GitAuthResolver
}

// NewDeployOperationHandler builds the handler to register with the deploy
// runtime.
//
// authResolverFor is a factory, not a resolver, and is called fresh inside
// Handle rather than once here: the resolver's GitHub App client is loaded
// (and cached process-wide) on first successful use, so building one eagerly
// at process startup -- before an operator has configured a GitHub App --
// would permanently bake in "no app configured" for every deploy this
// process ever runs, even after one is added later.
func NewDeployOperationHandler(log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, dockerClient *mobyclient.Client, authResolverFor func(context.Context) GitAuthResolver) workers.Handler {
	return &deployOperationHandler{log: log, cfg: cfg, store: store, sealer: sealer, dockerClient: dockerClient, authResolverFor: authResolverFor}
}

func (h *deployOperationHandler) Kind() string { return repository.DeployOperationKind }

func (h *deployOperationHandler) Handle(ctx context.Context, op workers.Operation) error {
	deployment, err := h.store.GetServiceDeployment(ctx, op.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// deployments CASCADE-deletes with its service; the operation row
			// can outlive it -- a service can be deleted while its deploy is
			// still queued behind another on the same lock_key.
			return workers.ErrOperationObsolete
		}
		return fmt.Errorf("read deployment: %w", err)
	}

	job, err := LoadDeployJob(ctx, h.cfg, h.store, deployment)
	if err != nil {
		return fmt.Errorf("load deploy job: %w", err)
	}

	// The runtime builds ctx itself and knows nothing about obs; without this
	// every deploy_steps row ExecuteDeploy writes is silently dropped. There
	// is no live HTTP request by the time a queued deploy is claimed, so the
	// operation id (== deployment id) stands in for a request id -- it is
	// the one stable identifier this execution attempt has.
	execCtx := obs.WithStore(reqctx.WithRequestID(ctx, op.ID), h.store)
	log := h.log.With("service_id", deployment.ServiceID, "environment_id", deployment.EnvironmentID, "deployment_id", deployment.ID)

	// Deliberately context.Background(), matching every pre-queue call site:
	// loading (and caching) the GitHub App client is process lifetime, not
	// this operation's, and must not be cancelled by this deploy's own lease
	// or cancellation.
	_, execErr := ExecuteDeploy(execCtx, log, h.cfg, h.store, job, h.sealer, h.dockerClient, h.authResolverFor(context.Background()))

	// ExecuteDeploy already writes its own terminal status onto deployments,
	// which the claim/complete projection (internal/repository/operations_deploy.go)
	// carries onto operations as a guarded no-op. Re-read rather than trust
	// execErr: on cancellation ExecuteDeploy resolves through CancelDeployment
	// rather than returning a plain context.Canceled the caller must
	// remember to special-case, so the row is the only reliable source of
	// what actually happened.
	completed, err := h.store.GetServiceDeployment(context.WithoutCancel(ctx), op.ID)
	if err != nil {
		return fmt.Errorf("read deployment outcome: %w", err)
	}
	switch completed.Status {
	case models.DeploymentSuccess, models.DeploymentCancelled:
		return nil
	case models.DeploymentFailed:
		code := FirstPublicCode(execErr)
		if code == "" || code == "internal_error" {
			code = "deploy_failed"
		}
		return &workers.HandlerError{Code: code, Err: errors.New(completed.ErrorMessage)}
	default:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return workers.Failf("deploy_incomplete",
			"deploy finished without recording an outcome (status %q)", completed.Status)
	}
}

// Ready re-reads deployments' status right before dispatch and refuses to
// start a deploy that is not (or no longer) QUEUED/BUILDING.
//
// CancelDeployment -- the guarded, synchronous UPDATE the HTTP cancel
// endpoint uses -- writes deployments directly and does not touch
// operations. If it commits between claim and dispatch, the operations row
// is still 'queued'/'running' and would be claimed and run with no other
// check in its way; this is what stops that.
func (h *deployOperationHandler) Ready(ctx context.Context, op workers.Operation) (time.Duration, error) {
	deployment, err := h.store.GetServiceDeployment(ctx, op.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, workers.ErrOperationObsolete
		}
		return 0, fmt.Errorf("read deployment: %w", err)
	}
	switch deployment.Status {
	case models.DeploymentQueued, models.DeploymentBuilding:
		return 0, nil
	default:
		return 0, workers.ErrOperationObsolete
	}
}
