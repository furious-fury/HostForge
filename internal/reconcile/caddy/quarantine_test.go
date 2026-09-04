package caddy

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/repository"
)

// newQuarantineTestReconciler builds a Reconciler against a real SQLite
// store and a scripted caddy binary whose validate/reload calls read back
// the rendered config and fail only when it contains one of badDomains --
// letting a test control which domain(s) the fleet render rejects, rather
// than only whether it succeeds at all.
func newQuarantineTestReconciler(t *testing.T, badDomains ...string) (r *Reconciler, store *repository.Store) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell stub for the caddy binary")
	}

	dbPath := filepath.Join(t.TempDir(), "hostforge.sqlite")
	db, err := database.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store = repository.New(db)

	dir := t.TempDir()
	bin := filepath.Join(dir, "caddy")
	var checks []string
	for _, domain := range badDomains {
		checks = append(checks, `if grep -q "`+domain+`" "$cfg"; then echo "parse error near `+domain+`" >&2; exit 1; fi`)
	}
	script := "#!/bin/sh\n" +
		"prev=\"\"\n" +
		"for a; do\n  if [ \"$prev\" = \"--config\" ]; then cfg=\"$a\"; fi\n  prev=\"$a\"\ndone\n" +
		strings.Join(checks, "\n") + "\n" +
		"exit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o750); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		CaddyBin:           bin,
		CaddyGeneratedPath: filepath.Join(dir, "hostforge.caddy"),
		CaddyRootConfig:    filepath.Join(dir, "Caddyfile"),
	}
	if err := os.WriteFile(cfg.CaddyRootConfig, []byte("import "+cfg.CaddyGeneratedPath+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	return New(discardLog(), cfg, store), store
}

// seedRoutableDomain creates one service with a running container and one
// custom domain pointed at it. Callers needing several domains ordered by
// updated_at should sleep briefly between calls: RFC3339 (what the
// repository stores) has one-second resolution.
func seedRoutableDomain(t *testing.T, store *repository.Store, hostname string) (applicationID, environmentID, domainID string) {
	t.Helper()
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Quarantine", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	env := envs[0]
	service, err := store.CreateService(ctx, repository.CreateServiceInput{ApplicationID: app.ID, Name: "svc-" + hostname, RepoURL: "https://github.com/acme/" + hostname + ".git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: env.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDeploymentStatus(ctx, deployment.ID, models.DeploymentSuccess, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttachContainer(ctx, repository.AttachContainerInput{DeploymentID: deployment.ID, DockerContainerID: "container-" + hostname, InternalPort: 3000, HostPort: 18080}); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateServiceDeployment(ctx, service.ID, env.ID, deployment.ID); err != nil {
		t.Fatal(err)
	}
	domain, err := store.CreateServiceDomain(ctx, app.ID, env.ID, service.ID, hostname)
	if err != nil {
		t.Fatal(err)
	}
	return app.ID, env.ID, domain.ID
}

func domainByID(t *testing.T, store *repository.Store, appID, envID, domainID string) repository.ServiceDomain {
	t.Helper()
	item, err := store.GetServiceDomain(context.Background(), appID, envID, domainID)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

// The core §19 guarantee: one malformed domain does not block the fleet.
// The bad domain is created last, so it is unambiguously the most
// recently changed candidate -- the culprit-selection heuristic.
func TestReconcileQuarantinesOneDomainAndPublishesTheRest(t *testing.T) {
	r, store := newQuarantineTestReconciler(t, "bad.invalid.test")
	appID, envID, goodID := seedRoutableDomain(t, store, "good.example.test")
	time.Sleep(1100 * time.Millisecond) // force a distinct, later updated_at
	_, _, badID := seedRoutableDomain(t, store, "bad.invalid.test")

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile returned an error; the good domain should still have published: %v", err)
	}
	if result.Routes != 1 {
		t.Fatalf("routes = %d, want 1 (only the surviving good domain)", result.Routes)
	}

	good := domainByID(t, store, appID, envID, goodID)
	if good.PublishState != models.PublishStatePublished {
		t.Fatalf("good domain publish_state = %q, want published", good.PublishState)
	}
	bad := domainByID(t, store, appID, envID, badID)
	if bad.PublishState != models.PublishStateInvalid {
		t.Fatalf("bad domain publish_state = %q, want invalid", bad.PublishState)
	}
	if !strings.Contains(bad.PublishError, "bad.invalid.test") {
		t.Fatalf("publish_error missing the validator output: %q", bad.PublishError)
	}
}

// Editing an invalid domain is how it re-enters the fleet: the repository
// resets publish_state to unpublished on any domain_name update
// (internal/repository/domains_v2.go), which is what makes it a candidate
// again on the next Reconcile.
func TestEditingAnInvalidDomainReadmitsItToTheFleet(t *testing.T) {
	r, store := newQuarantineTestReconciler(t, "bad.invalid.test")
	appID, envID, goodID := seedRoutableDomain(t, store, "good.example.test")
	time.Sleep(1100 * time.Millisecond)
	_, _, badID := seedRoutableDomain(t, store, "bad.invalid.test")

	if _, err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if domainByID(t, store, appID, envID, badID).PublishState != models.PublishStateInvalid {
		t.Fatal("setup: domain was not quarantined")
	}

	fixed := domainByID(t, store, appID, envID, badID)
	if _, err := store.UpdateServiceDomain(context.Background(), appID, envID, badID, "fixed.example.test", fixed.ServiceID); err != nil {
		t.Fatal(err)
	}
	if got := domainByID(t, store, appID, envID, badID); got.PublishState != models.PublishStateUnpublished {
		t.Fatalf("publish_state after edit = %q, want unpublished immediately (before any reconcile)", got.PublishState)
	}

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("the corrected domain should reconcile cleanly now: %v", err)
	}
	if result.Routes != 2 {
		t.Fatalf("routes = %d, want 2: the corrected domain should be back in the fleet", result.Routes)
	}
	if got := domainByID(t, store, appID, envID, badID); got.PublishState != models.PublishStatePublished {
		t.Fatalf("corrected domain publish_state = %q, want published", got.PublishState)
	}
	if got := domainByID(t, store, appID, envID, goodID); got.PublishState != models.PublishStatePublished {
		t.Fatalf("the unrelated good domain should have stayed published throughout: %q", got.PublishState)
	}
}

// The quarantine loop must terminate rather than walking the whole fleet
// one at a time when several domains fail together for an unrelated
// reason -- everything remaining goes back to unpublished instead.
func TestReconcileQuarantineCapTerminates(t *testing.T) {
	names := make([]string, 0, maxQuarantinesPerPass+2)
	for i := 0; i < maxQuarantinesPerPass+2; i++ {
		names = append(names, "bad-"+string(rune('a'+i))+".invalid.test")
	}
	r, store := newQuarantineTestReconciler(t, names...)
	appID, envID := "", ""
	ids := make([]string, 0, len(names))
	for i, name := range names {
		if i > 0 {
			time.Sleep(1100 * time.Millisecond)
		}
		a, e, id := seedRoutableDomain(t, store, name)
		appID, envID = a, e
		ids = append(ids, id)
	}

	result, err := r.Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected Reconcile to give up and return an error")
	}
	if result.Routes != 0 {
		t.Fatalf("routes = %d after giving up, want 0", result.Routes)
	}

	invalidCount, unpublishedCount := 0, 0
	for _, id := range ids {
		switch domainByID(t, store, appID, envID, id).PublishState {
		case models.PublishStateInvalid:
			invalidCount++
		case models.PublishStateUnpublished:
			unpublishedCount++
		}
	}
	if invalidCount != maxQuarantinesPerPass {
		t.Fatalf("quarantined = %d, want exactly the cap (%d)", invalidCount, maxQuarantinesPerPass)
	}
	if unpublishedCount != len(names)-maxQuarantinesPerPass {
		t.Fatalf("unpublished = %d, want %d (everyone past the cap)", unpublishedCount, len(names)-maxQuarantinesPerPass)
	}
}

// Add-time validation itself (caddy.ValidateSiteBlock, called from the
// domain-create/update handlers) is tested directly in
// internal/caddy/runtime_test.go -- no need to duplicate it here.
