package workers

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/furious-fury/HostForge/internal/repository"
)

// run is one worker: drain the queue, then wait and drain again.
func (r *Runtime) run(stopCtx context.Context, owner string) {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		for {
			if stopCtx.Err() != nil {
				return
			}
			operation, err := r.cfg.Store.ClaimNextOperation(stopCtx, repository.ClaimOptions{
				Owner:       owner,
				Lease:       r.cfg.Lease,
				MinPriority: r.cfg.MinPriority,
			})
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			if err != nil {
				if stopCtx.Err() != nil {
					return
				}
				r.cfg.Log.Error("claim operation failed", "worker", owner, "error", err)
				break
			}
			r.execute(stopCtx, owner, operation)
		}
		select {
		case <-stopCtx.Done():
			return
		case <-ticker.C:
		}
	}
}

// execute runs one claimed operation to a terminal state.
func (r *Runtime) execute(stopCtx context.Context, owner string, operation repository.Operation) {
	log := r.cfg.Log.With("operation_id", operation.ID, "kind", operation.Kind, "worker", owner)

	handler, ok := r.registry.lookup(operation.Kind)
	if !ok {
		log.Error("no handler registered for operation kind")
		r.complete(stopCtx, owner, operation, "failed", "", "operation_kind_not_registered",
			"no handler is registered for kind "+operation.Kind)
		return
	}

	// Deliberately detached from stopCtx. A claimed operation holds a lease
	// and owns real resources, so shutdown drains it rather than cancelling
	// it mid-step; only losing the lease or an explicit cancellation request
	// stops it early.
	operationCtx, cancel := context.WithCancel(context.WithoutCancel(stopCtx))
	defer cancel()

	if checker, ok := handler.(ReadinessChecker); ok {
		retryAfter, err := checker.Ready(operationCtx, operation)
		switch {
		case errors.Is(err, ErrOperationObsolete):
			log.Info("operation is no longer applicable; cancelling")
			r.complete(stopCtx, owner, operation, "cancelled", "obsolete", "", "")
			return
		case err != nil:
			log.Error("operation readiness check failed", "error", err)
			r.complete(stopCtx, owner, operation, "failed", "", errorCode(err), err.Error())
			return
		case retryAfter > 0:
			// Not an attempt: DeferOperation gives back the one the claim took.
			if err := r.cfg.Store.DeferOperation(stopCtx, operation.ID, owner, retryAfter); err != nil {
				log.Error("defer operation failed", "error", err)
			}
			return
		}
	}

	r.trackLease(operation.ID, owner)
	defer r.releaseLease(operation.ID)

	leaseDone := make(chan struct{})
	go func() {
		defer close(leaseDone)
		r.holdLease(operationCtx, cancel, owner, operation.ID, log)
	}()

	err := handler.Handle(operationCtx, operation)
	cancel()
	<-leaseDone

	switch {
	case err == nil:
		r.complete(stopCtx, owner, operation, "success", "", "", "")
	case errors.Is(err, context.Canceled):
		// The context is cancelled by cancellation request or lease loss.
		// Either way the operation stopped at a step boundary; keep whatever
		// progress step it had reached.
		log.Info("operation cancelled")
		r.complete(stopCtx, owner, operation, "cancelled", "", "", "")
	default:
		log.Error("operation failed", "error", err)
		r.complete(stopCtx, owner, operation, "failed", "", errorCode(err), err.Error())
	}
}

// holdLease renews the lease until the operation finishes, and cancels it if
// the lease is lost or cancellation is requested. Renewal and the
// cancellation check are one query, so the two facts are always consistent.
func (r *Runtime) holdLease(operationCtx context.Context, cancel context.CancelFunc, owner, id string, log logger) {
	ticker := time.NewTicker(r.cfg.LeaseRefresh)
	defer ticker.Stop()
	for {
		select {
		case <-operationCtx.Done():
			return
		case <-ticker.C:
			cancelRequested, err := r.cfg.Store.RenewOperationLease(operationCtx, id, owner, r.cfg.Lease)
			if err != nil {
				if operationCtx.Err() != nil {
					return
				}
				log.Error("operation lease lost; stopping work", "error", err)
				cancel()
				return
			}
			if cancelRequested {
				log.Info("cancellation requested; stopping at the next step boundary")
				cancel()
				return
			}
		}
	}
}

// complete records a terminal status. It uses stopCtx rather than the
// operation context, which is cancelled by the time this runs on the
// cancellation path — writing the outcome must not itself be cancelled.
func (r *Runtime) complete(stopCtx context.Context, owner string, operation repository.Operation, status, progressStep, errorCode, errorMessage string) {
	ctx := context.WithoutCancel(stopCtx)
	if err := r.cfg.Store.CompleteOperation(ctx, repository.CompleteOperationInput{
		ID:           operation.ID,
		Owner:        owner,
		Status:       status,
		ProgressStep: progressStep,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	}); err != nil && !errors.Is(err, sql.ErrNoRows) {
		r.cfg.Log.Error("record operation completion failed",
			"operation_id", operation.ID, "status", status, "error", err)
	}
}

func errorCode(err error) string {
	var handlerErr *HandlerError
	if errors.As(err, &handlerErr) && handlerErr.Code != "" {
		return handlerErr.Code
	}
	return "operation_failed"
}

// logger is the subset of *slog.Logger holdLease uses, so it can be given a
// logger already scoped to the operation.
type logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}
