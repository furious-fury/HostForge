// Package obs attaches optional persistence hooks for the observability UI (SQLite samples).
package obs

import (
	"context"
	"log/slog"

	"github.com/hostforge/hostforge/internal/models"
)

// ObservabilityWriter is the subset of persistence used for best-effort UI samples.
// Implemented by *repository.Store without this package importing repository (avoids
// gopls/import cycles and keeps obs usable from the persistence layer).
type ObservabilityWriter interface {
	InsertDeployStep(ctx context.Context, in models.DeployStepRecord) error
	InsertHTTPRequest(ctx context.Context, in models.HTTPRequestRecord) error
}

type storeKey struct{}

// WithStore returns ctx that carries an ObservabilityWriter for best-effort observability inserts.
func WithStore(ctx context.Context, store ObservabilityWriter) context.Context {
	if store == nil {
		return ctx
	}
	return context.WithValue(ctx, storeKey{}, store)
}

// StoreFrom returns the writer attached by WithStore, or nil.
func StoreFrom(ctx context.Context) ObservabilityWriter {
	v, _ := ctx.Value(storeKey{}).(ObservabilityWriter)
	return v
}

// RecordDeployStep persists a deploy or system span; failures are logged and ignored.
func RecordDeployStep(ctx context.Context, log *slog.Logger, in models.DeployStepRecord) {
	st := StoreFrom(ctx)
	if st == nil {
		return
	}
	if err := st.InsertDeployStep(ctx, in); err != nil && log != nil {
		log.Warn("observability deploy_step insert failed", "error", err, "step", in.Step)
	}
}

// RecordHTTPRequest persists an HTTP sample; failures are logged and ignored.
func RecordHTTPRequest(ctx context.Context, log *slog.Logger, in models.HTTPRequestRecord) {
	st := StoreFrom(ctx)
	if st == nil {
		return
	}
	if err := st.InsertHTTPRequest(ctx, in); err != nil && log != nil {
		log.Warn("observability http_request insert failed", "error", err)
	}
}
