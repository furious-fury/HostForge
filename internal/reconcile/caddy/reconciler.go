package caddy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	caddyruntime "github.com/furious-fury/HostForge/internal/caddy"
	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/repository"
)

// Result reports what one reconcile pass did, for logging and for tests
// that need to assert the no-op path without inspecting internal state.
type Result struct {
	Applied bool // Caddy was reloaded (or the render didn't need one -- see Skipped)
	Routes  int  // routable domains in the rendered config
	Skipped bool // rendered content was unchanged since the last successful apply
}

// Reconciler owns converging Caddy's running configuration onto the domains
// the database says should be routed. Call Notify after any write that
// changes desired state (a domain created/updated/deleted, a deploy cutover);
// the periodic tick in Start is the self-healing backstop for a failed
// reload or a manual edit outside HostForge, not the primary trigger.
//
// Reconcile is exported and safe to call directly, for callers that need a
// synchronous answer (an operator-triggered "sync now", or a caller that
// must block until a route is live before proceeding). That is a deliberate
// departure from ADR-0002 §6.2, which describes a single reconciling
// goroutine and therefore no mutex: this package has more than one call
// path into the critical section, so a sync.Mutex serializes them instead.
// The properties §6.2 actually cares about -- no interleaved reload, no
// redundant apply -- hold either way.
type Reconciler struct {
	log   *slog.Logger
	cfg   *config.Config
	store *repository.Store

	notify chan struct{} // buffered, size 1; non-blocking send from Notify

	mu       sync.Mutex
	lastHash string // sha256 of the last successfully applied render; empty forces a render
}

// New builds a Reconciler. Call Start to run its loop; Reconcile and Notify
// work without Start too, which is what lets synchronous callers (an
// operator's "sync now") use the same instance the loop uses.
func New(log *slog.Logger, cfg *config.Config, store *repository.Store) *Reconciler {
	return &Reconciler{log: log, cfg: cfg, store: store, notify: make(chan struct{}, 1)}
}

// Notify asks the reconciler to converge soon. Never blocks: a full send
// buffer means a reconcile is already pending, so this notification would
// be redundant. Twenty deploys finishing together produce one extra pass,
// not twenty (ADR-0002 §6.2 "burst coalescing").
func (r *Reconciler) Notify() {
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

// Start runs the reconcile loop until ctx is done: once immediately (so boot
// converges without waiting for the first tick or the first notify), then on
// whichever comes first of a notify or the periodic tick. A non-positive
// interval disables the periodic tick; Notify-driven reconciles still run.
func (r *Reconciler) Start(ctx context.Context) {
	go func() {
		if _, err := r.Reconcile(ctx); err != nil {
			r.log.Warn("caddy reconcile failed", "error", err)
		}
		interval := time.Duration(r.cfg.CaddyReconcileIntervalSeconds) * time.Second
		if interval <= 0 {
			interval = 0
		}
		var tick <-chan time.Time
		if interval > 0 {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			tick = ticker.C
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.notify:
			case <-tick:
			}
			if _, err := r.Reconcile(ctx); err != nil {
				r.log.Warn("caddy reconcile failed", "error", err)
			}
		}
	}()
}

// Reconcile renders the desired routes from the database, skips the
// write/validate/reload cycle entirely if nothing changed since the last
// successful apply, and otherwise writes, validates, and reloads Caddy --
// recording the outcome on each domain's publish_state rather than failing
// any deployment. A domain with no upstream yet (no active deployment, or
// its container isn't RUNNING) is unpublished, not an error: it simply has
// nothing to route to.
//
// PR2 (ADR-0002 §19) adds per-domain quarantine on top of this: today a
// single malformed domain fails the whole render and every domain goes
// unpublished together. That is worse than the eventual §19 behaviour but
// strictly better than the deploy-path defect this package replaces --
// nothing here can fail a deployment or roll back a binding.
func (r *Reconciler) Reconcile(ctx context.Context) (Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	domainRoutes, err := r.store.ListDomainRoutes(ctx)
	if err != nil {
		return Result{}, err
	}

	var routes []caddyruntime.Route
	var routableIDs []string
	var notYetIDs []string
	for _, dr := range domainRoutes {
		if dr.HostPort <= 0 {
			notYetIDs = append(notYetIDs, dr.ID)
			continue
		}
		routes = append(routes, caddyruntime.Route{Domain: dr.DomainName, HostPort: dr.HostPort})
		routableIDs = append(routableIDs, dr.ID)
	}
	// Marking "not yet routable" domains is cheap and idempotent, so it runs
	// on every pass regardless of the hash skip below -- a newly created
	// domain must never wait behind an unrelated no-op render to be marked.
	if err := r.store.SetDomainsPublishState(ctx, notYetIDs, models.PublishStateUnpublished); err != nil {
		r.log.Warn("failed to mark unroutable domains unpublished", "count", len(notYetIDs), "error", err)
	}

	var certificateDomains []string
	if endpoint, endpointErr := r.store.GetDatabaseGatewayEndpoint(ctx, "postgresql"); endpointErr == nil && endpoint.DesiredStatus == "active" {
		certificateDomains = append(certificateDomains, endpoint.Hostname)
	}

	content := caddyruntime.RenderConfigWithCertificateDomains(routes, certificateDomains)
	hash := sha256Hex(content)
	if hash == r.lastHash {
		return Result{Applied: true, Routes: len(routes), Skipped: true}, nil
	}

	syncRes, err := caddyruntime.Sync(ctx, caddyruntime.SyncOptions{
		CaddyBin:           r.cfg.CaddyBin,
		GeneratedPath:      r.cfg.CaddyGeneratedPath,
		RootConfig:         r.cfg.CaddyRootConfig,
		Routes:             routes,
		CertificateDomains: certificateDomains,
	})
	if err != nil {
		if markErr := r.store.SetDomainsPublishState(ctx, routableIDs, models.PublishStateUnpublished); markErr != nil {
			r.log.Warn("failed to mark domains unpublished after sync failure", "count", len(routableIDs), "error", markErr)
		}
		return Result{Routes: len(routes)}, err
	}
	if !syncRes.Applied {
		r.log.Warn("caddy reload skipped (admin API unreachable); snippet written and validated -- start caddy, or it will apply on the next successful reconcile")
	}
	if err := r.store.SetDomainsPublishState(ctx, routableIDs, models.PublishStatePublished); err != nil {
		r.log.Warn("failed to mark domains published after sync success", "count", len(routableIDs), "error", err)
	}
	r.lastHash = hash
	r.log.Info("caddy reconcile complete", "routes", len(routes), "applied", syncRes.Applied)
	return Result{Applied: syncRes.Applied, Routes: len(routes)}, nil
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
