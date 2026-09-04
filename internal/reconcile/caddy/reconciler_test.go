package caddy

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/repository"
)

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

// newTestReconciler builds a Reconciler against a real SQLite store and a
// scripted caddy binary that appends its argv to invocationsPath on every
// call, so a test can assert not just outcomes but how many times -- or
// whether at all -- the binary actually ran. That is the only observable
// signal for the hash-skip guarantee.
func newTestReconciler(t *testing.T, exitCode int) (r *Reconciler, store *repository.Store, invocationsPath string) {
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
	invocationsPath = filepath.Join(dir, "invocations.log")
	bin := filepath.Join(dir, "caddy")
	script := "#!/bin/sh\necho \"$@\" >> " + invocationsPath + "\nexit " + strconv.Itoa(exitCode) + "\n"
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

	return New(discardLog(), cfg, store), store, invocationsPath
}

// seedDomain creates one service with a successful deployment and a custom
// domain pointed at it. hostPort <= 0 leaves no container attached, so
// ListDomainRoutes reports it as not-yet-routable -- the "deployed but no
// running container" state a fresh service is in before its first cutover.
func seedDomain(t *testing.T, store *repository.Store, hostPort int) (applicationID, environmentID, domainID string) {
	t.Helper()
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Recon", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	env := envs[0]
	service, err := store.CreateService(ctx, repository.CreateServiceInput{ApplicationID: app.ID, Name: "web", RepoURL: "https://github.com/acme/web.git", InternalPort: 3000})
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
	if hostPort > 0 {
		if _, err := store.AttachContainer(ctx, repository.AttachContainerInput{DeploymentID: deployment.ID, DockerContainerID: "container-1", InternalPort: 3000, HostPort: hostPort}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ActivateServiceDeployment(ctx, service.ID, env.ID, deployment.ID); err != nil {
		t.Fatal(err)
	}
	domain, err := store.CreateServiceDomain(ctx, app.ID, env.ID, service.ID, "web.example.test")
	if err != nil {
		t.Fatal(err)
	}
	return app.ID, env.ID, domain.ID
}

func domainState(t *testing.T, store *repository.Store, appID, envID, domainID string) repository.ServiceDomain {
	t.Helper()
	item, err := store.GetServiceDomain(context.Background(), appID, envID, domainID)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestReconcileMarksRoutableDomainPublishedOnSuccess(t *testing.T) {
	r, store, invocations := newTestReconciler(t, 0)
	appID, envID, domainID := seedDomain(t, store, 18080)

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.Routes != 1 || result.Skipped {
		t.Fatalf("unexpected result: %+v", result)
	}
	state := domainState(t, store, appID, envID, domainID)
	if state.PublishState != models.PublishStatePublished {
		t.Fatalf("publish_state = %q, want published", state.PublishState)
	}
	if state.SSLStatus != models.SSLStatusPending {
		t.Fatalf("ssl_status = %q, reconcile must not touch it", state.SSLStatus)
	}

	log, err := os.ReadFile(invocations)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "validate") || !strings.Contains(string(log), "reload") {
		t.Fatalf("expected validate and reload calls, got:\n%s", log)
	}
}

func TestReconcileSkipsUnchangedRender(t *testing.T) {
	r, store, invocations := newTestReconciler(t, 0)
	seedDomain(t, store, 18080)
	ctx := context.Background()

	first, err := r.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Skipped {
		t.Fatal("first reconcile must not be a skip -- nothing has been applied yet")
	}
	firstLog, err := os.ReadFile(invocations)
	if err != nil {
		t.Fatal(err)
	}

	second, err := r.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Skipped {
		t.Fatal("second reconcile with unchanged state must be a skip")
	}
	secondLog, err := os.ReadFile(invocations)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstLog) != string(secondLog) {
		t.Fatalf("skip still invoked the caddy binary:\nfirst:\n%s\nsecond:\n%s", firstLog, secondLog)
	}
}

func TestReconcileMarksDomainUnpublishedOnSyncFailure(t *testing.T) {
	r, store, _ := newTestReconciler(t, 1) // validate always fails
	appID, envID, domainID := seedDomain(t, store, 18080)

	if _, err := r.Reconcile(context.Background()); err == nil {
		t.Fatal("expected reconcile to report the sync failure")
	}
	state := domainState(t, store, appID, envID, domainID)
	if state.PublishState != models.PublishStateUnpublished {
		t.Fatalf("publish_state = %q, want unpublished", state.PublishState)
	}
	if state.SSLStatus != models.SSLStatusPending {
		t.Fatalf("ssl_status = %q, a routing failure must not be reported as a certificate error", state.SSLStatus)
	}
}

// A domain with no running container is not an error -- it simply has
// nothing to route to yet. This is the central fix PR1 makes to
// SyncCaddyRoutes's old behaviour, which marked exactly this case ssl_status
// ERROR.
func TestReconcileMarksUnroutableDomainUnpublishedNotError(t *testing.T) {
	r, store, _ := newTestReconciler(t, 0)
	appID, envID, domainID := seedDomain(t, store, 0)

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Routes != 0 {
		t.Fatalf("routes = %d, want 0: this domain has no upstream", result.Routes)
	}
	state := domainState(t, store, appID, envID, domainID)
	if state.PublishState != models.PublishStateUnpublished {
		t.Fatalf("publish_state = %q, want unpublished", state.PublishState)
	}
	if state.SSLStatus != models.SSLStatusPending {
		t.Fatalf("ssl_status = %q, want untouched PENDING -- not an error", state.SSLStatus)
	}
}

func TestNotifyNeverBlocks(t *testing.T) {
	r := New(discardLog(), &config.Config{}, nil)
	done := make(chan struct{})
	go func() {
		r.Notify()
		r.Notify()
		r.Notify()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify blocked -- the buffered send is no longer non-blocking")
	}
}
