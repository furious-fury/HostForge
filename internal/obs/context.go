// Package obs attaches optional persistence hooks for the observability UI (SQLite samples).
package obs

import (
	"context"
	"log/slog"

	"github.com/hostforge/hostforge/internal/repository"
)

type storeKey struct{}

// WithStore returns ctx that carries the repository Store for best-effort observability inserts.
func WithStore(ctx context.Context, store *repository.Store) context.Context {
	if store == nil {
		return ctx
	}
	return context.WithValue(ctx, storeKey{}, store)
}

// StoreFrom returns the store attached by WithStore, or nil.
func StoreFrom(ctx context.Context) *repository.Store {
	v, _ := ctx.Value(storeKey{}).(*repository.Store)
	return v
}

// RecordDeployStep persists a deploy or system span; failures are logged and ignored.
func RecordDeployStep(ctx context.Context, log *slog.Logger, in repository.DeployStepRecord) {
	st := StoreFrom(ctx)
	if st == nil {
		return
	}
	if err := st.InsertDeployStep(ctx, in); err != nil && log != nil {
		log.Warn("observability deploy_step insert failed", "error", err, "step", in.Step)
	}
}

// RecordHTTPRequest persists an HTTP sample; failures are logged and ignored.
func RecordHTTPRequest(ctx context.Context, log *slog.Logger, in repository.HTTPRequestRecord) {
	st := StoreFrom(ctx)
	if st == nil {
		return
	}
	if err := st.InsertHTTPRequest(ctx, in); err != nil && log != nil {
		log.Warn("observability http_request insert failed", "error", err)
	}
}
