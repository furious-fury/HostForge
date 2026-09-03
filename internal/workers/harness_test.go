package workers

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/repository"
)

// The harness runs against a real SQLite file, matching how every other
// store-touching test in this repo works, and a hand-written handler fake.
// There is no Docker here at all — keeping the runtime ignorant of the
// domain is what makes that possible.

type testRuntime struct {
	t       *testing.T
	store   *repository.Store
	dbPath  string
	runtime *Runtime
	handler *recordingHandler
	cancel  context.CancelFunc
}

// exec runs SQL directly against the same database file. repository.Store's
// handle is unexported, so a test in this package that needs to stage a row
// state the public API cannot produce opens its own connection.
//
// That second connection races the runtime's own writes (the lease-refresh
// ticker in particular) for SQLite's single write lock. `_busy_timeout=`
// is the mattn/go-sqlite3 spelling; modernc.org/sqlite (this driver, see
// internal/database.OpenSQLite) silently discards it, leaving the timeout
// at SQLite's default of zero -- any contention then fails immediately
// with SQLITE_BUSY instead of waiting. That was rare enough to miss under
// a normal test run, but reliably surfaced under `go test -race`, which
// slows execution enough to widen the collision window. Use the pragma
// form OpenSQLite documents so this connection actually waits like every
// other one in the codebase does.
func (h *testRuntime) exec(query string, args ...any) {
	h.t.Helper()
	raw, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", filepath.ToSlash(h.dbPath)))
	if err != nil {
		h.t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.ExecContext(context.Background(), query, args...); err != nil {
		h.t.Fatal(err)
	}
}

type runtimeOption func(*Config)

func withConcurrency(n int) runtimeOption {
	return func(cfg *Config) { cfg.Concurrency = n }
}

func withLeaseRefresh(d time.Duration) runtimeOption {
	return func(cfg *Config) { cfg.LeaseRefresh = d }
}

func withPollInterval(d time.Duration) runtimeOption {
	return func(cfg *Config) { cfg.PollInterval = d }
}

func withCancelPoll(d time.Duration) runtimeOption {
	return func(cfg *Config) { cfg.CancelPoll = d }
}

func newTestRuntime(t *testing.T, handler *recordingHandler, opts ...runtimeOption) *testRuntime {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "hostforge.sqlite")
	db, err := database.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)

	cfg := Config{
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:        store,
		Concurrency:  1,
		PollInterval: 10 * time.Millisecond,
		LeaseRefresh: 20 * time.Millisecond,
		Lease:        2 * time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if handler != nil {
		if err := runtime.Register(handler); err != nil {
			t.Fatal(err)
		}
	}
	return &testRuntime{t: t, store: store, dbPath: dbPath, runtime: runtime, handler: handler}
}

func (h *testRuntime) start() {
	h.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.t.Cleanup(cancel)
	if err := h.runtime.Start(ctx); err != nil {
		h.t.Fatal(err)
	}
}

func (h *testRuntime) enqueue(id, lockKey string) repository.Operation {
	h.t.Helper()
	return h.enqueueKind(id, "test_kind", lockKey)
}

func (h *testRuntime) enqueueKind(id, kind, lockKey string) repository.Operation {
	h.t.Helper()
	operation, err := h.store.EnqueueOperation(context.Background(), repository.NewOperationInput{
		ID: id, Kind: kind, LockKey: lockKey, ServiceID: "svc-1",
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return operation
}

// newSharedTestStore opens a store two runtimes can share, for tests that
// need to prove one runtime does not observe or disturb another's claims.
func newSharedTestStore(t *testing.T) *repository.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "hostforge.sqlite")
	db, err := database.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return repository.New(db)
}

// newRuntimeOverStore builds a runtime against an existing store, for tests
// with more than one runtime sharing one table.
func newRuntimeOverStore(t *testing.T, store *repository.Store, handler *recordingHandler, opts ...runtimeOption) *testRuntime {
	t.Helper()
	cfg := Config{
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:        store,
		Concurrency:  1,
		PollInterval: 10 * time.Millisecond,
		LeaseRefresh: 20 * time.Millisecond,
		Lease:        2 * time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if handler != nil {
		if err := runtime.Register(handler); err != nil {
			t.Fatal(err)
		}
	}
	return &testRuntime{t: t, store: store, runtime: runtime, handler: handler}
}

func withSkipRecovery() runtimeOption {
	return func(cfg *Config) { cfg.SkipRecovery = true }
}

// awaitStatus polls until the operation reaches want, or fails the test.
func (h *testRuntime) awaitStatus(id, want string) repository.Operation {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last repository.Operation
	for time.Now().Before(deadline) {
		operation, err := h.store.GetOperation(context.Background(), id)
		if err != nil {
			h.t.Fatal(err)
		}
		last = operation
		if operation.Status == want {
			return operation
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("operation %s is %q, want %q (progress_step=%q error=%q)",
		id, last.Status, want, last.ProgressStep, last.ErrorMessage)
	return last
}

// recordingHandler is a hand-written fake in the style of
// fakePostgreSQLGatewayRuntime in internal/services: no mocking library, no
// codegen, just recorded calls and injectable behaviour.
type recordingHandler struct {
	kind   string
	calls  atomic.Int32
	handle func(context.Context, Operation) error
	ready  func(context.Context, Operation) (time.Duration, error)

	mu      sync.Mutex
	handled []string
	// steps receives a value at each step boundary a handler reaches, for
	// asserting where cancellation landed.
	steps chan string
}

func newRecordingHandler() *recordingHandler {
	return &recordingHandler{kind: "test_kind", steps: make(chan string, 16)}
}

func (h *recordingHandler) Kind() string { return h.kind }

func (h *recordingHandler) Handle(ctx context.Context, op Operation) error {
	h.calls.Add(1)
	h.mu.Lock()
	h.handled = append(h.handled, op.ID)
	h.mu.Unlock()
	if h.handle != nil {
		return h.handle(ctx, op)
	}
	return nil
}

func (h *recordingHandler) handledIDs() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.handled...)
}

// readinessHandler adds the optional ReadinessChecker capability. It is a
// separate type so a plain recordingHandler does not accidentally satisfy
// the interface.
type readinessHandler struct {
	*recordingHandler
}

func (h readinessHandler) Ready(ctx context.Context, op Operation) (time.Duration, error) {
	if h.recordingHandler.ready != nil {
		return h.recordingHandler.ready(ctx, op)
	}
	return 0, nil
}
