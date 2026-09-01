package workers

import (
	"context"
	"time"

	"github.com/furious-fury/HostForge/internal/repository"
)

// Store is the queue persistence the runtime needs, declared here rather
// than in repository so this package depends only on what it uses — the
// same shape as serviceMetricStore in cmd/server. *repository.Store
// satisfies it without knowing this interface exists.
type Store interface {
	ClaimNextOperation(ctx context.Context, opts repository.ClaimOptions) (repository.Operation, error)
	RenewOperationLease(ctx context.Context, id, owner string, lease time.Duration) (cancelRequested bool, err error)
	DeferOperation(ctx context.Context, id, owner string, retryAfter time.Duration) error
	CompleteOperation(ctx context.Context, in repository.CompleteOperationInput) error
	RecoverOperations(ctx context.Context, at time.Time) (requeued int64, failed int64, err error)
}
