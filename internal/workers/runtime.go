package workers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/furious-fury/HostForge/internal/repository"
)

// Defaults match the timings the database operation worker has always used,
// so the port changes no observable pacing.
const (
	defaultLease        = 2 * time.Minute
	defaultLeaseRefresh = 30 * time.Second
	defaultPollInterval = 2 * time.Second
	// Cancellation is observed on its own clock, not the lease clock. One
	// second keeps "stop" responsive without meaningfully adding load: it is
	// a primary-key read per running operation.
	defaultCancelPoll = time.Second
)

// Config configures a Runtime. Only Store and Concurrency are required.
type Config struct {
	Log         *slog.Logger
	Store       Store
	Concurrency int

	// Lease is how long a claim is held; LeaseRefresh how often it is
	// renewed.
	Lease        time.Duration
	LeaseRefresh time.Duration

	// CancelPoll is how often a running operation is checked for a
	// cancellation request. Deliberately far shorter than LeaseRefresh:
	// renewing a lease is a transaction, but noticing a cancellation is one
	// indexed read, and an operator pressing cancel should not wait a lease
	// interval to be heard.
	CancelPoll time.Duration

	// PollInterval is how long a worker waits after finding the queue empty.
	PollInterval time.Duration

	// MinPriority reserves this runtime's workers for operations at or above
	// a priority. Zero claims anything (ADR-0002 §20.2).
	MinPriority int

	// SkipRecovery skips the RecoverOperations call in Start. Recovery is
	// process-wide, not per-runtime: it requeues or fails every abandoned
	// operation regardless of kind, so when a process runs more than one
	// Runtime over the same store, exactly one of them may recover — running
	// it twice races each runtime's in-flight claims against the other's
	// recovery sweep, and a second sweep after workers have already started
	// claiming can snatch back work that is legitimately in progress. The
	// runtime that does NOT skip it must be started first.
	SkipRecovery bool
}

// Runtime owns a pool of workers claiming from the operations queue.
type Runtime struct {
	cfg      Config
	registry *registry
	wg       sync.WaitGroup

	// kinds is a snapshot of the registered handler kinds, taken once in
	// Start after Register can no longer be called. Reading it from run
	// needs no lock: it is written once before any worker goroutine starts,
	// and never written again.
	kinds []string

	mu      sync.Mutex
	started bool
	leases  map[string]heldLease // operation id -> what's needed to release it on a timed-out drain
}

// heldLease is what releaseHeldLeases needs about a claim it did not itself
// make: who holds it, and whether giving the attempt back is even sound.
type heldLease struct {
	owner       string
	maxAttempts int
}

func New(cfg Config) (*Runtime, error) {
	if cfg.Store == nil {
		return nil, errors.New("workers: a store is required")
	}
	if cfg.Concurrency < 1 {
		return nil, fmt.Errorf("workers: concurrency must be positive, got %d", cfg.Concurrency)
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	if cfg.Lease <= 0 {
		cfg.Lease = defaultLease
	}
	if cfg.LeaseRefresh <= 0 {
		cfg.LeaseRefresh = defaultLeaseRefresh
	}
	if cfg.CancelPoll <= 0 {
		cfg.CancelPoll = defaultCancelPoll
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	return &Runtime{cfg: cfg, registry: newRegistry(), leases: map[string]heldLease{}}, nil
}

// Register adds handlers. It must be called before Start.
func (r *Runtime) Register(handlers ...Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return errors.New("workers: handlers must be registered before Start")
	}
	for _, handler := range handlers {
		if err := r.registry.register(handler); err != nil {
			return err
		}
	}
	return nil
}

// Start recovers abandoned operations and then launches the workers.
//
// Recovery runs synchronously here, before any worker exists, which is the
// point: it used to live in a separate loop that callers had to remember to
// invoke first, an ordering that was load-bearing and unwritten. There is no
// longer a second call to order.
//
// stopCtx signals shutdown. Workers stop claiming when it is cancelled;
// operations already claimed run on their own context and are drained by
// Wait.
func (r *Runtime) Start(stopCtx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return errors.New("workers: already started")
	}
	r.started = true
	r.kinds = r.registry.kinds()
	r.mu.Unlock()

	if r.cfg.SkipRecovery {
		r.cfg.Log.Info("skipping operation recovery; another runtime in this process owns it")
	} else {
		requeued, failed, err := r.cfg.Store.RecoverOperations(stopCtx, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("recover operations: %w", err)
		}
		if requeued > 0 || failed > 0 {
			r.cfg.Log.Info("recovered interrupted operations", "requeued", requeued, "failed", failed)
		}
	}

	for index := 0; index < r.cfg.Concurrency; index++ {
		owner, err := workerID(index)
		if err != nil {
			return fmt.Errorf("generate worker identity: %w", err)
		}
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.run(stopCtx, owner)
		}()
	}
	return nil
}

// Wait blocks until every worker has stopped, or until ctx expires.
//
// On expiry it releases the operations still running. Which way depends on
// whether the operation can be retried at all (ADR-0002 §20.1):
//
//   - MaxAttempts > 1: deferred back to the queue, so the next startup
//     recovers it immediately instead of waiting out a lease that will never
//     be renewed.
//   - MaxAttempts <= 1: completed as failed/interrupted instead. Deferring
//     gives the attempt back (DeferOperation's whole point is that a
//     deferral is not an attempt), so a non-resumable operation deferred
//     here would be reclaimed and re-run on the next boot — which is exactly
//     what a max_attempts=1 operation exists to forbid. Recording the
//     interruption is the honest outcome: the operation did not finish, and
//     it will not be retried.
//
// Either way the operation itself is not cancelled — the process is about to
// exit, and a half-applied operation is better recovered or recorded than
// interrupted mid-write.
func (r *Runtime) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		r.releaseHeldLeases(context.WithoutCancel(ctx))
		return ctx.Err()
	}
}

func (r *Runtime) releaseHeldLeases(ctx context.Context) {
	r.mu.Lock()
	held := make(map[string]heldLease, len(r.leases))
	for id, lease := range r.leases {
		held[id] = lease
	}
	r.mu.Unlock()

	for id, lease := range held {
		if lease.maxAttempts <= 1 {
			if err := r.cfg.Store.CompleteOperation(ctx, repository.CompleteOperationInput{
				ID: id, Owner: lease.owner, Status: "failed",
				ErrorCode: "interrupted", ErrorMessage: "operation interrupted by shutdown",
			}); err != nil {
				r.cfg.Log.Warn("fail non-resumable operation on shutdown failed", "operation_id", id, "error", err)
				continue
			}
			r.cfg.Log.Warn("operation interrupted by shutdown; not retried", "operation_id", id)
			continue
		}
		if err := r.cfg.Store.DeferOperation(ctx, id, lease.owner, 0); err != nil {
			r.cfg.Log.Warn("release operation lease on shutdown failed", "operation_id", id, "error", err)
			continue
		}
		r.cfg.Log.Info("released operation lease for recovery on next start", "operation_id", id)
	}
}

func (r *Runtime) trackLease(operation repository.Operation, owner string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.leases[operation.ID] = heldLease{owner: owner, maxAttempts: operation.MaxAttempts}
}

func (r *Runtime) releaseLease(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.leases, id)
}

func workerID(index int) (string, error) {
	token := make([]byte, 12)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	return fmt.Sprintf("hostforge-%d-%d-%s", os.Getpid(), index, hex.EncodeToString(token)), nil
}
