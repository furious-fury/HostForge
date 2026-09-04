package caddy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// maxQuarantinesPerPass bounds how many domains one Reconcile call will
// quarantine before giving up and marking whatever remains unpublished
// instead. Cheap insurance against a pathological render that keeps
// failing for a reason unrelated to any single domain -- the loop
// terminates instead of walking the whole fleet one quarantine at a time.
const maxQuarantinesPerPass = 5

// reconcileCandidate is one domain still in contention for this pass: its
// row id (to write publish_state back), the route it would render as, and
// updatedAt, which is how a validation failure is attributed to a domain
// (ADR-0002 §19.3: the most-recently-changed candidate is presumed guilty).
type reconcileCandidate struct {
	id        string
	route     caddyruntime.Route
	updatedAt time.Time
}

// Reconcile renders the desired routes from the database, skips the
// write/validate/reload cycle entirely if nothing changed since the last
// successful apply, and otherwise writes, validates, and reloads Caddy --
// recording the outcome on each domain's publish_state rather than failing
// any deployment. A domain with no upstream yet (no active deployment, or
// its container isn't RUNNING) is unpublished, not an error: it simply has
// nothing to route to.
//
// A domain already marked invalid is excluded from the render entirely,
// rather than being retried every pass only to fail the same way -- it
// re-enters once its own row changes (an edit resets it to unpublished,
// internal/repository/domains_v2.go).
//
// If the render still fails validation with other domains involved, that
// is quarantine territory (ADR-0002 §19): the most-recently-changed
// candidate is presumed guilty, marked invalid with the validator's
// output, excluded, and the remainder retried -- up to
// maxQuarantinesPerPass times -- so one malformed domain cannot block
// every other domain from publishing. A reload failure (Caddy not
// running, an unrelated process error) is not a domain problem and never
// quarantines anything; every candidate just goes back to unpublished for
// the next pass to retry as a whole.
func (r *Reconciler) Reconcile(ctx context.Context) (Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	domainRoutes, err := r.store.ListDomainRoutes(ctx)
	if err != nil {
		return Result{}, err
	}

	var candidates []reconcileCandidate
	var notYetIDs []string
	for _, dr := range domainRoutes {
		if dr.PublishState == models.PublishStateInvalid {
			continue
		}
		if dr.HostPort <= 0 {
			notYetIDs = append(notYetIDs, dr.ID)
			continue
		}
		candidates = append(candidates, reconcileCandidate{
			id:        dr.ID,
			route:     caddyruntime.Route{Domain: dr.DomainName, HostPort: dr.HostPort},
			updatedAt: dr.UpdatedAt,
		})
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

	hash := sha256Hex(renderCandidates(candidates, certificateDomains))
	if hash == r.lastHash {
		return Result{Applied: true, Routes: len(candidates), Skipped: true}, nil
	}

	quarantined := 0
	for {
		routes := routesOf(candidates)
		syncRes, syncErr := caddyruntime.Sync(ctx, caddyruntime.SyncOptions{
			CaddyBin:           r.cfg.CaddyBin,
			GeneratedPath:      r.cfg.CaddyGeneratedPath,
			RootConfig:         r.cfg.CaddyRootConfig,
			Routes:             routes,
			CertificateDomains: certificateDomains,
		})
		if syncErr == nil {
			if !syncRes.Applied {
				r.log.Warn("caddy reload skipped (admin API unreachable); snippet written and validated -- start caddy, or it will apply on the next successful reconcile")
			}
			if err := r.store.SetDomainsPublishState(ctx, idsOf(candidates), models.PublishStatePublished); err != nil {
				r.log.Warn("failed to mark domains published after sync success", "count", len(candidates), "error", err)
			}
			r.lastHash = sha256Hex(renderCandidates(candidates, certificateDomains))
			r.log.Info("caddy reconcile complete", "routes", len(candidates), "applied", syncRes.Applied, "quarantined", quarantined)
			return Result{Applied: syncRes.Applied, Routes: len(candidates)}, nil
		}

		culprit, ok := worstOffender(candidates)
		if !errors.Is(syncErr, caddyruntime.ErrValidationFailed) || !ok || quarantined >= maxQuarantinesPerPass {
			if quarantined >= maxQuarantinesPerPass {
				r.log.Warn("caddy reconcile gave up quarantining; too many domains still fail validation together", "quarantined", quarantined, "remaining", len(candidates), "error", syncErr)
			}
			if markErr := r.store.SetDomainsPublishState(ctx, idsOf(candidates), models.PublishStateUnpublished); markErr != nil {
				r.log.Warn("failed to mark domains unpublished after sync failure", "count", len(candidates), "error", markErr)
			}
			return Result{Routes: len(candidates)}, syncErr
		}

		if err := r.store.SetDomainPublishState(ctx, culprit.id, models.PublishStateInvalid, syncErr.Error()); err != nil {
			r.log.Warn("failed to quarantine invalid domain", "domain", culprit.route.Domain, "error", err)
		} else {
			r.log.Warn("quarantined invalid domain; retrying the rest of the fleet", "domain", culprit.route.Domain, "validator_error", syncErr)
		}
		candidates = without(candidates, culprit.id)
		quarantined++
	}
}

func renderCandidates(candidates []reconcileCandidate, certificateDomains []string) string {
	return caddyruntime.RenderConfigWithCertificateDomains(routesOf(candidates), certificateDomains)
}

func routesOf(candidates []reconcileCandidate) []caddyruntime.Route {
	routes := make([]caddyruntime.Route, len(candidates))
	for i, c := range candidates {
		routes[i] = c.route
	}
	return routes
}

func idsOf(candidates []reconcileCandidate) []string {
	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.id
	}
	return ids
}

// worstOffender returns the candidate with the greatest updatedAt -- the
// one most likely to be responsible for a fleet-wide validation failure,
// since a set that validated at the end of the last successful pass only
// breaks when something in it just changed (ADR-0002 §19.3).
func worstOffender(candidates []reconcileCandidate) (reconcileCandidate, bool) {
	if len(candidates) == 0 {
		return reconcileCandidate{}, false
	}
	worst := candidates[0]
	for _, c := range candidates[1:] {
		if c.updatedAt.After(worst.updatedAt) {
			worst = c
		}
	}
	return worst, true
}

func without(candidates []reconcileCandidate, id string) []reconcileCandidate {
	out := make([]reconcileCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.id != id {
			out = append(out, c)
		}
	}
	return out
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
