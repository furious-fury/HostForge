// Package obs attaches optional persistence hooks for the observability UI (SQLite samples).
package obs

import (
	"context"
	"log/slog"
	"time"

	"github.com/furious-fury/HostForge/internal/models"
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

// writeTimeout bounds a detached observability insert. Long enough to outlast
// SQLite's busy timeout, short enough that a wedged write cannot hold up
// shutdown.
const writeTimeout = 5 * time.Second

// writeContext detaches a record from the operation it describes.
//
// These writes used to inherit the caller's context, which meant a cancelled
// deploy, or a client that hung up, cancelled the record of itself: the driver
// rejected the insert and the only trace left was a WARN line. That is
// backwards. A cancellation is when the record matters most -- it is the sole
// evidence of where the work stopped and how long the teardown took.
//
// context.WithoutCancel keeps the values, which is load-bearing here: the
// writer itself rides on the context, so dropping values would turn every
// record into a silent no-op.
func writeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
}

// RecordDeployStep persists a deploy or system span; failures are logged and ignored.
func RecordDeployStep(ctx context.Context, log *slog.Logger, in models.DeployStepRecord) {
	st := StoreFrom(ctx)
	if st == nil {
		return
	}
	writeCtx, cancel := writeContext(ctx)
	defer cancel()
	if err := st.InsertDeployStep(writeCtx, in); err != nil && log != nil {
		log.Warn("observability deploy_step insert failed", "error", err, "step", in.Step)
	}
}

// RecordHTTPRequest persists an HTTP sample; failures are logged and ignored.
func RecordHTTPRequest(ctx context.Context, log *slog.Logger, in models.HTTPRequestRecord) {
	st := StoreFrom(ctx)
	if st == nil {
		return
	}
	writeCtx, cancel := writeContext(ctx)
	defer cancel()
	if err := st.InsertHTTPRequest(writeCtx, in); err != nil && log != nil {
		log.Warn("observability http_request insert failed", "error", err)
	}
}
