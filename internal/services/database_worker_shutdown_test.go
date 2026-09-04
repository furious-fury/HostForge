package services

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/repository"
)

func newWorkerTestStore(t *testing.T) *repository.Store {
	t.Helper()
	db, err := database.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "hostforge.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return repository.New(db)
}

func newWorkerTestSealer(t *testing.T) *envcrypt.Sealer {
	t.Helper()
	sealer, err := envcrypt.NewFromBase64Key(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return sealer
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// countingHandler counts emitted log records, so a test can observe how many
// times a loop iterated without adding a seam to production code.
type countingHandler struct {
	mu    sync.Mutex
	count int
}

func (h *countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *countingHandler) Handle(context.Context, slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.count++
	return nil
}
func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(string) slog.Handler      { return h }
func (h *countingHandler) total() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

// A persistent claim error is not ErrNoRows, so the gateway worker's error
// branch used to fall through to a bare continue: no sleep, no yield, a hot
// loop for as long as the error lasted. Closing the database reproduces that
// exactly — every claim fails with something that is not ErrNoRows.
//
// The assertion is on iteration count, not on the worker returning: the
// stopCtx check at the top of the loop makes it return either way, so a test
// that only waits for it to exit passes against the unfixed code. Each
// failing claim logs once, so counting log records counts iterations.
func TestGatewayWorkerBacksOffOnPersistentClaimError(t *testing.T) {
	t.Parallel()
	db, err := database.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "gateway.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	store := repository.New(db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	counter := &countingHandler{}
	stopCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runDatabaseGatewayWorker(stopCtx, slog.New(counter), &config.Config{}, store,
			newWorkerTestSealer(t), nil, "worker-test", nil)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("gateway worker never returned; it is ignoring the stop context")
	}

	// With the 2s backoff ticker the worker logs once and then blocks until
	// the 250ms context expires. Without it, an unthrottled loop against a
	// closed database logs thousands of times in the same window.
	if got := counter.total(); got > 5 {
		t.Fatalf("gateway worker logged %d claim errors in 250ms; it is spinning rather than backing off", got)
	}
}
