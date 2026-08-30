package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/furious-fury/HostForge/internal/auth"
	"github.com/furious-fury/HostForge/internal/bootstrap"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/hostmetrics"
	"github.com/furious-fury/HostForge/internal/logging"
	logsapi "github.com/furious-fury/HostForge/internal/logs"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/obs"
	"github.com/furious-fury/HostForge/internal/redact"
	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/furious-fury/HostForge/internal/reqctx"
	"github.com/furious-fury/HostForge/internal/services"
	"github.com/moby/moby/client"
)

// serverStartedAt is used for uptime in /api/settings.
var serverStartedAt = time.Now().UTC()

func main() {
	log := logging.New()
	code := runServer(log, os.Args[1:])
	os.Exit(code)
}

func runServer(log *slog.Logger, args []string) int {
	defaultListen := strings.TrimSpace(os.Getenv(config.ListenEnv))
	if defaultListen == "" {
		defaultListen = ":8080"
	}
	defaultWebhookPath := strings.TrimSpace(os.Getenv(config.WebhookBasePathEnv))
	if defaultWebhookPath == "" {
		defaultWebhookPath = "/hooks/github"
	}
	defaultWebhookBodyLimit := cfgIntDefault(config.WebhookMaxBodyBytesEnv, 1_048_576)
	defaultWebhookAsync := cfgBoolDefault(config.WebhookAsyncEnv, false)

	fs := flagSet("server")
	dataDir := fs.String("data-dir", "", "data directory (overrides "+config.DataDirEnv+")")
	listen := fs.String("listen", defaultListen, "listen address (overrides "+config.ListenEnv+")")
	webhookPath := fs.String("webhook-path", defaultWebhookPath, "github webhook route path (overrides "+config.WebhookBasePathEnv+")")
	webhookMaxBodyBytes := fs.Int("webhook-max-body-bytes", defaultWebhookBodyLimit, "max webhook payload body in bytes (overrides "+config.WebhookMaxBodyBytesEnv+")")
	webhookAsync := fs.Bool("webhook-async", defaultWebhookAsync, "accept and process webhooks asynchronously (overrides "+config.WebhookAsyncEnv+")")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: config: %v\n", err)
		return 1
	}
	cfg.ListenAddr = strings.TrimSpace(*listen)
	if err := (bootstrap.Config{Enabled: cfg.BootstrapEnabled, PublicIP: cfg.BootstrapPublicIP, HTTPSPort: cfg.BootstrapHTTPSPort, ExpiresAt: cfg.BootstrapExpiresAt}).Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error: bootstrap config: %v\n", err)
		return 2
	}
	if cfg.BootstrapEnabled {
		host, _, splitErr := net.SplitHostPort(cfg.ListenAddr)
		if splitErr != nil || !(host == "127.0.0.1" || host == "::1" || strings.EqualFold(host, "localhost")) {
			fmt.Fprintln(os.Stderr, "error: bootstrap mode requires HOSTFORGE_LISTEN to be loopback-only")
			return 2
		}
	}
	cfg.WebhookBasePath = normalizeRoutePath(*webhookPath)
	cfg.WebhookMaxBodyBytes = *webhookMaxBodyBytes
	cfg.WebhookAsync = *webhookAsync
	if cfg.ListenAddr == "" {
		fmt.Fprintln(os.Stderr, "error: listen address must not be empty")
		return 2
	}
	if strings.TrimSpace(cfg.APIToken) == "" {
		fmt.Fprintf(os.Stderr, "error: %s must be set\n", config.APITokenEnv)
		return 2
	}
	if strings.TrimSpace(cfg.SessionSecret) == "" {
		fmt.Fprintf(os.Stderr, "error: %s must be set\n", config.SessionSecretEnv)
		return 2
	}
	if len(strings.TrimSpace(cfg.SessionSecret)) < 16 {
		fmt.Fprintf(os.Stderr, "error: %s must be at least 16 characters\n", config.SessionSecretEnv)
		return 2
	}
	if strings.TrimSpace(cfg.SessionCookieName) == "" {
		fmt.Fprintln(os.Stderr, "error: session cookie name must not be empty")
		return 2
	}
	if cfg.SessionTTLMinutes <= 0 {
		fmt.Fprintln(os.Stderr, "error: session ttl minutes must be > 0")
		return 2
	}
	if strings.TrimSpace(cfg.WebhookSecret) == "" {
		fmt.Fprintf(os.Stderr, "error: %s must be set\n", config.WebhookSecretEnv)
		return 2
	}
	if strings.TrimSpace(os.Getenv(config.EnvEncryptionKeyEnv)) == "" {
		fmt.Fprintf(os.Stderr, "error: %s is required\n", config.EnvEncryptionKeyEnv)
		return 2
	}
	if cfg.WebhookRateLimitPerMinute <= 0 {
		fmt.Fprintln(os.Stderr, "error: webhook rate limit per minute must be > 0")
		return 2
	}
	if cfg.LoginRateLimitPerMinute <= 0 {
		fmt.Fprintln(os.Stderr, "error: login rate limit per minute must be > 0")
		return 2
	}
	if cfg.ShutdownTimeoutSeconds <= 0 {
		fmt.Fprintln(os.Stderr, "error: shutdown timeout seconds must be > 0")
		return 2
	}
	if cfg.WebhookMaxBodyBytes <= 0 {
		fmt.Fprintln(os.Stderr, "error: webhook max body bytes must be > 0")
		return 2
	}
	if err := services.ValidateRuntimeConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: runtime config: %v\n", err)
		return 2
	}

	for _, d := range []string{cfg.DataDir, cfg.WorktreesDir(), cfg.BuildsDir(), cfg.LogsDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error: mkdir %s: %v\n", d, err)
			return 1
		}
	}

	ctx := context.Background()
	if err := services.ValidateRailpackReadiness(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: railpack readiness: %v\n", err)
		return 2
	}
	db, err := database.OpenSQLite(ctx, cfg.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: sqlite: %v\n", err)
		return 1
	}
	defer db.Close()

	// One Docker client is shared by every deploy, provisioning, and metrics path
	// instead of each constructing and closing its own (ADR-0002 §8.2). Deferred
	// after db.Close() so it closes first on shutdown (defers run LIFO).
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: docker: %v\n", err)
		return 1
	}
	defer dockerClient.Close()

	store := repository.New(db)
	if created, err := store.EnsureActivePlatformServiceDomains(ctx); err != nil {
		log.Warn("backfill platform share domains failed", "error", err)
	} else if created > 0 {
		if err := services.SyncCaddyRoutes(ctx, log, cfg, store); err != nil {
			log.Warn("sync backfilled platform share domains failed", "created", created, "error", err)
		} else {
			log.Info("backfilled platform share domains", "created", created)
		}
	}
	// shutdownCtx is cancelled on SIGINT/SIGTERM. Every background loop below
	// takes it directly and stops on its own the moment it's cancelled — the
	// same context that tells us a signal arrived is what tells the loops to
	// stop, so there is no separate "now cancel the loops" step later. The
	// HTTP listener does not use this ctx directly (http.Server has no such
	// parameter); it's drained explicitly via Shutdown() below instead.
	shutdownCtx, stopSignalNotify := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignalNotify()

	services.StartCaddyCertPollLoop(shutdownCtx, log, cfg, store, obs.WithStore(context.Background(), store))
	startServiceMetricSampler(shutdownCtx, log, store, dockerClient)
	webhookLimiter := newFixedWindowLimiter(cfg.WebhookRateLimitPerMinute, time.Minute)
	loginLimiter := newFixedWindowLimiter(cfg.LoginRateLimitPerMinute, time.Minute)

	hostReader := hostmetrics.DefaultReader(hostmetrics.ParseReaderOptionsFromEnv())
	hostSampler := hostmetrics.NewSampler(hostmetrics.IntervalFromEnv(5000), hostmetrics.CapacityFromEnv(360), hostReader)
	hostSampler.Start(shutdownCtx)

	// HOSTFORGE_ENV_ENCRYPTION_KEY presence was already checked above; construct
	// the sealer and confirm it's the same key that sealed this database's
	// existing secrets before anything else runs (ADR-0002 §20.4). A mismatched
	// key is not recoverable — every secret it sealed is permanently unreadable
	// with any other key — so this must be a startup failure, not a degraded
	// feature.
	envSealer, err := envcrypt.NewFromBase64Key(strings.TrimSpace(os.Getenv(config.EnvEncryptionKeyEnv)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s: %v\n", config.EnvEncryptionKeyEnv, err)
		return 1
	}
	if err := envcrypt.VerifyOrInitCanary(envSealer,
		func() ([]byte, bool, error) { return store.GetEncryptionCanary(ctx) },
		func(sealed []byte) error { return store.SetEncryptionCanary(ctx, sealed) },
	); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	services.StartDatabaseReconciliationLoop(shutdownCtx, log, store, envSealer, dockerClient)
	services.StartDatabaseOperationLoop(shutdownCtx, log, store, envSealer, dockerClient, cfg.DataDir, cfg.DatabaseMinFreeDiskBytes, cfg.DatabaseOperationConcurrency, cfg)
	services.StartDatabasePurgeLoop(shutdownCtx, log, store, dockerClient)
	services.StartDatabaseBackupScheduleLoop(shutdownCtx, log, store, cfg.DatabaseTransferMaxPerHour)
	services.StartDatabaseBackupRetentionLoop(shutdownCtx, log, store, envSealer)
	services.StartDatabaseGatewayOperationLoop(shutdownCtx, log, cfg, store, envSealer, dockerClient)

	handler := &server{
		log:            log,
		cfg:            cfg,
		store:          store,
		webhookLimiter: webhookLimiter,
		loginLimiter:   loginLimiter,
		hostSampler:    hostSampler,
		envSealer:      envSealer,
		dockerClient:   dockerClient,
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.WebhookBasePath, handler.withRequestContext(handler.handleGitHubWebhook))
	mux.HandleFunc("/auth/session", handler.withRequestContext(handler.handleSessionRoutes))
	mux.HandleFunc("/api/system/status", handler.withRequestContext(handler.requireManagementAuth(handler.handleSystemStatus)))
	mux.HandleFunc("/api/onboarding", handler.withRequestContext(handler.requireManagementAuth(handler.handleOnboardingRoutes)))
	mux.HandleFunc("/api/system/host/snapshot", handler.withRequestContext(handler.requireManagementAuth(handler.handleHostSnapshot)))
	mux.HandleFunc("/api/system/host/history", handler.withRequestContext(handler.requireManagementAuth(handler.handleHostHistory)))
	mux.HandleFunc("/api/settings", handler.withRequestContext(handler.requireManagementAuth(handler.handleSettingsRoutes)))
	mux.HandleFunc("/api/settings/", handler.withRequestContext(handler.requireManagementAuth(handler.handleSettingsRoutes)))
	mux.HandleFunc("/api/repositories/branches", handler.withRequestContext(handler.requireManagementAuth(handler.handleRepositoryBranches)))
	mux.HandleFunc("/api/github/", handler.withRequestContext(handler.requireManagementAuth(handler.handleGitHubAppRoutes)))
	mux.HandleFunc("/api/applications", handler.withRequestContext(handler.requireManagementAuth(handler.handleApplications)))
	mux.HandleFunc("/api/applications/", handler.withRequestContext(handler.requireManagementAuth(handler.handleApplications)))
	mux.HandleFunc("/api/database-engines", handler.withRequestContext(handler.requireManagementAuth(handler.handleDatabaseEngines)))
	mux.HandleFunc("/api/database-gateways/", handler.withRequestContext(handler.requireManagementAuth(handler.handleDatabaseGateways)))
	mux.HandleFunc("/api/database-gateway-operations/", handler.withRequestContext(handler.requireManagementAuth(handler.handleDatabaseGatewayOperations)))
	mux.HandleFunc("/api/database-external-connections/", handler.withRequestContext(handler.requireManagementAuth(handler.handleDatabaseExternalConnections)))
	mux.HandleFunc("/api/backup-destinations", handler.withRequestContext(handler.requireManagementAuth(handler.handleBackupDestinations)))
	mux.HandleFunc("/api/backup-destinations/", handler.withRequestContext(handler.requireManagementAuth(handler.handleBackupDestinations)))
	mux.HandleFunc("/api/database-instances/", handler.withRequestContext(handler.requireManagementAuth(handler.handleDatabaseInstances)))
	mux.HandleFunc("/api/database-backups/", handler.withRequestContext(handler.requireManagementAuth(handler.handleDatabaseBackups)))
	mux.HandleFunc("/api/database-bindings/", handler.withRequestContext(handler.requireManagementAuth(handler.handleDatabaseBindings)))
	mux.HandleFunc("/api/database-operations/", handler.withRequestContext(handler.requireManagementAuth(handler.handleDatabaseOperations)))
	mux.HandleFunc("/api/database-services/", handler.withRequestContext(handler.requireManagementAuth(handler.handleDatabaseServices)))
	mux.HandleFunc("/api/services/", handler.withRequestContext(handler.requireManagementAuth(handler.handleServices)))
	mux.HandleFunc("/api/events", handler.withRequestContext(handler.requireManagementAuth(handler.handlePlatformEvents)))
	mux.HandleFunc("/api/deployments", handler.withRequestContext(handler.requireManagementAuth(handler.handleDeploymentsV2Collection)))
	mux.HandleFunc("/api/deployments/", handler.withRequestContext(handler.requireManagementAuth(handler.handleDeploymentRoutes)))
	mux.HandleFunc("/api/observability/", handler.withRequestContext(handler.requireManagementAuth(handler.handleObservabilityRoutes)))
	registerStaticUIRoutes(mux, log)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       0,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		log.Info("hostforge server listening", "listen", cfg.ListenAddr, "webhook_path", cfg.WebhookBasePath, "webhook_async", cfg.WebhookAsync)
		serveErr <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "error: server: %v\n", err)
			return 1
		}
		return 0
	case <-shutdownCtx.Done():
		log.Info("shutdown: signal received")
	}

	// Bounds both drains below. Background loops above already stopped (or are
	// stopping) on their own, since they share shutdownCtx directly.
	drainCtx, cancelDrain := context.WithTimeout(context.Background(), time.Duration(cfg.ShutdownTimeoutSeconds)*time.Second)
	defer cancelDrain()

	log.Info("shutdown: stopping http listener")
	if err := httpServer.Shutdown(drainCtx); err != nil {
		log.Warn("shutdown: http listener did not drain cleanly", "error", err)
	}

	// Deploys run in goroutines detached from the request that launched them
	// (webhook and manual/redeploy/rollback handlers all do this), so
	// httpServer.Shutdown above — which only waits on active request
	// handlers — does not wait for them. Wait on them explicitly instead, so
	// an in-flight build isn't killed mid-step by a restart.
	log.Info("shutdown: draining in-flight deploys")
	deploysDone := make(chan struct{})
	go func() {
		handler.deployWG.Wait()
		close(deploysDone)
	}()
	select {
	case <-deploysDone:
		log.Info("shutdown: deploys drained")
	case <-drainCtx.Done():
		log.Warn("shutdown: deploy drain timed out; exiting with deploys still in flight")
	}

	// dockerClient and db close via their defers above, in that order.
	log.Info("shutdown: complete")
	return 0
}

type server struct {
	log                     *slog.Logger
	cfg                     *config.Config
	store                   *repository.Store
	webhookLimiter          *fixedWindowLimiter
	loginLimiter            *fixedWindowLimiter
	hostSampler             *hostmetrics.Sampler
	hostSnapCache           hostSnapshotCache
	envSealer               *envcrypt.Sealer
	appCache                *appClientHolder
	githubRepoLister        githubRepositoryLister
	deploymentCancelMu      sync.Mutex
	databaseGatewayDomainMu sync.RWMutex
	deploymentCancels       map[string]context.CancelFunc
	dockerClient            *client.Client
	deployWG                sync.WaitGroup
}

type githubPushPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
}

func (s *server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	requestID := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery"))
	if requestID == "" {
		requestID = reqctx.RequestID(r.Context())
	}
	if requestID == "" {
		requestID = newRequestID()
	}
	ctx := reqctx.WithRequestID(r.Context(), requestID)
	r = r.WithContext(ctx)
	log := s.requestLog(r)
	remoteIP := requestIP(r)

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"status":     "error",
			"request_id": requestID,
			"error":      "method_not_allowed",
		})
		return
	}
	if !strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
			"status":     "error",
			"request_id": requestID,
			"error":      "content_type_must_be_application_json",
		})
		return
	}
	if !s.webhookLimiter.Allow(remoteIP, time.Now().UTC()) {
		log.Warn("webhook rejected", "reason", "rate_limited", "remote_ip", remoteIP)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"status":     "error",
			"request_id": requestID,
			"error":      "rate_limited",
		})
		return
	}

	eventType := strings.TrimSpace(r.Header.Get("X-GitHub-Event"))

	r.Body = http.MaxBytesReader(w, r.Body, int64(s.cfg.WebhookMaxBodyBytes))
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status":     "error",
			"request_id": requestID,
			"error":      "invalid_request_body",
		})
		return
	}
	signature := strings.TrimSpace(r.Header.Get("X-Hub-Signature-256"))
	if signature == "" {
		log.Warn("webhook rejected", "reason", "missing_signature", "remote_ip", remoteIP)
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"status":     "error",
			"request_id": requestID,
			"error":      "missing_signature",
		})
		return
	}
	if !s.verifyWebhookSignature(r.Context(), signature, body) {
		log.Warn("webhook rejected", "reason", "signature_mismatch", "remote_ip", remoteIP)
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"status":     "error",
			"request_id": requestID,
			"error":      "invalid_signature",
		})
		return
	}

	switch eventType {
	case "installation", "installation_repositories":
		s.handleInstallationEvent(r.Context(), log, eventType, body)
		writeJSON(w, http.StatusOK, map[string]string{
			"status":     "ok",
			"request_id": requestID,
			"event":      eventType,
		})
		return
	case "", "push":
		// fall through to push handling below
	default:
		log.Info("ignoring unsupported webhook event", "event", eventType)
		writeJSON(w, http.StatusOK, map[string]string{
			"status":     "ignored",
			"request_id": requestID,
			"reason":     "unsupported_event",
		})
		return
	}

	var payload githubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status":     "error",
			"request_id": requestID,
			"error":      "invalid_json_payload",
		})
		return
	}

	repoURL, err := services.CanonicalRepoURL(payload.Repository.CloneURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status":     "error",
			"request_id": requestID,
			"error":      "invalid_repository_clone_url",
		})
		return
	}
	branch, ok := branchFromRef(payload.Ref)
	if !ok {
		log.Info("ignoring non-branch push ref", "ref", payload.Ref)
		writeJSON(w, http.StatusOK, map[string]string{
			"status":     "ignored",
			"request_id": requestID,
			"reason":     "non_branch_ref",
		})
		return
	}
	if isZeroSHA(payload.After) {
		log.Info("ignoring push payload with zero commit sha", "ref", payload.Ref)
		writeJSON(w, http.StatusOK, map[string]string{
			"status":     "ignored",
			"request_id": requestID,
			"reason":     "deleted_ref",
		})
		return
	}

	targets, err := s.store.ListAutoDeployTargets(r.Context())
	if err != nil {
		log.Error("service environment lookup failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"status": "error", "request_id": requestID, "error": "service_environment_lookup_failed",
		})
		return
	}
	matches := make([]repository.AutoDeployTarget, 0)
	for _, candidate := range targets {
		canonical, canonicalErr := services.CanonicalRepoURL(candidate.RepoURL)
		if canonicalErr == nil && canonical == repoURL && candidate.Branch == branch {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		log.Info("ignoring push without matching auto-deploy binding", "repo_url", redact.RepoURLForLog(repoURL), "branch", branch)
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ignored", "request_id": requestID, "reason": "no_matching_service_environment",
		})
		return
	}

	deploymentIDs := make([]string, 0, len(matches))
	for _, match := range matches {
		target, resolveErr := services.ResolveDeployTarget(r.Context(), s.store, match.ServiceID, match.EnvironmentID)
		if resolveErr != nil {
			log.Error("matched service environment could not be resolved", "service_id", match.ServiceID, "environment_id", match.EnvironmentID, "error", resolveErr)
			continue
		}
		job, prepareErr := services.PrepareServiceDeploy(r.Context(), s.cfg, s.store, target, "github_push", "github", strings.TrimSpace(payload.After), "")
		if prepareErr != nil {
			log.Error("failed to accept webhook deployment", "service_id", match.ServiceID, "environment_id", match.EnvironmentID, "error", prepareErr)
			continue
		}
		deploymentIDs = append(deploymentIDs, job.Deployment.ID)
		ctx, cancel := context.WithCancel(context.Background())
		s.registerDeploymentCancel(job.Deployment.ID, cancel)
		deployLog := log.With("service_id", match.ServiceID, "environment_id", match.EnvironmentID, "deployment_id", job.Deployment.ID, "repo_url", redact.RepoURLForLog(repoURL), "branch", branch)
		s.deployWG.Add(1)
		go func(job services.DeployJob, deployLog *slog.Logger) {
			defer s.deployWG.Done()
			defer s.unregisterDeploymentCancel(job.Deployment.ID)
			bg := obs.WithStore(ctx, s.store)
			_, execErr := services.ExecuteDeploy(bg, deployLog, s.cfg, s.store, job, s.envSealer, s.dockerClient, s.newGitAuthResolver(context.Background()))
			if execErr != nil && !errors.Is(execErr, context.Canceled) {
				deployLog.Error("async webhook deployment failed", "error", execErr)
			}
		}(job, deployLog)
	}
	if len(deploymentIDs) == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"status": "error", "request_id": requestID, "error": "failed_to_accept_deployments",
		})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "accepted", "request_id": requestID, "deployment_ids": deploymentIDs, "count": len(deploymentIDs), "mode": "async",
	})
}

func (s *server) handleSessionRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleSessionCreate(w, r)
	case http.MethodGet:
		s.handleSessionStatus(w, r)
	case http.MethodDelete:
		s.handleSessionDelete(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
	}
}

func (s *server) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	remoteIP := requestIP(r)
	if !s.loginLimiter.Allow(remoteIP, time.Now().UTC()) {
		s.requestLog(r).Warn("login rejected", "reason", "rate_limited", "remote_ip", remoteIP)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"status": "error", "error": "rate_limited"})
		return
	}
	if !auth.BearerMatches(r.Header.Get("Authorization"), s.cfg.APIToken) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"status": "error", "error": "invalid_api_token"})
		return
	}
	ttl := time.Duration(s.cfg.SessionTTLMinutes) * time.Minute
	sessionValue, _, err := auth.NewSignedSession(s.cfg.SessionSecret, ttl)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "session_create_failed"})
		return
	}
	s.setSessionCookie(w, sessionValue, ttl)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "authenticated": true})
}

func (s *server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authenticateRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "authenticated": false, "error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "authenticated": true})
}

func (s *server) handleSessionDelete(w http.ResponseWriter, _ *http.Request) {
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "authenticated": false})
}

func (s *server) requireManagementAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := s.authenticateRequest(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"status": "error", "error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

type responseWriterStatus struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriterStatus) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterStatus) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not implement http.Hijacker")
	}
	return hj.Hijack()
}

func (rw *responseWriterStatus) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *server) withRequestContext(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if rid == "" {
			rid = newRequestID()
		}
		ctx := reqctx.WithRequestID(r.Context(), rid)
		ctx = obs.WithStore(ctx, s.store)
		r = r.WithContext(ctx)
		start := time.Now()
		rw := &responseWriterStatus{ResponseWriter: w, status: http.StatusOK}
		next(rw, r)
		dur := time.Since(start).Milliseconds()
		applicationID, serviceID, environmentID := s.requestResourceScope(r.Context(), r.URL.Path)
		s.log.Info("http_request", "request_id", rid, "method", r.Method, "path", r.URL.Path, "status", rw.status, "duration_ms", dur)
		obs.RecordHTTPRequest(r.Context(), s.log, models.HTTPRequestRecord{
			RequestID: rid, ApplicationID: applicationID, ServiceID: serviceID, EnvironmentID: environmentID,
			Method: r.Method, Path: r.URL.Path, Status: rw.status, DurationMS: dur, StartedAt: start.UTC(),
		})
	}
}

func (s *server) requestResourceScope(ctx context.Context, path string) (applicationID, serviceID, environmentID string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "api" {
		return "", "", ""
	}
	switch parts[1] {
	case "applications":
		applicationID = parts[2]
		if len(parts) >= 5 && parts[3] == "environments" {
			environmentID = parts[4]
		}
	case "services":
		serviceID = parts[2]
		if service, err := s.store.GetService(ctx, serviceID); err == nil {
			applicationID = service.ApplicationID
		}
		if len(parts) >= 5 && parts[3] == "environments" {
			environmentID = parts[4]
		}
	case "deployments":
		if deployment, err := s.store.GetServiceDeployment(ctx, parts[2]); err == nil {
			serviceID, environmentID = deployment.ServiceID, deployment.EnvironmentID
			if service, err := s.store.GetService(ctx, serviceID); err == nil {
				applicationID = service.ApplicationID
			}
		}
	}
	return applicationID, serviceID, environmentID
}

func (s *server) requestLog(r *http.Request) *slog.Logger {
	if r == nil {
		return s.log
	}
	if id := reqctx.RequestID(r.Context()); id != "" {
		return s.log.With("request_id", id)
	}
	return s.log
}

func publicAPIError(err error, fallback string) string {
	if err == nil {
		return ""
	}
	code := services.FirstPublicCode(err)
	if code == "" || code == "internal_error" {
		return fallback
	}
	return code
}

func (s *server) authenticateRequest(r *http.Request) (string, bool) {
	if auth.BearerMatches(r.Header.Get("Authorization"), s.cfg.APIToken) {
		return "bearer", true
	}
	cookie, err := r.Cookie(s.cfg.SessionCookieName)
	if err == nil {
		if _, verifyErr := auth.VerifySignedSession(s.cfg.SessionSecret, cookie.Value, time.Now().UTC()); verifyErr == nil {
			return "session", true
		}
	}
	return "", false
}

func (s *server) setSessionCookie(w http.ResponseWriter, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.cfg.SessionCookieSecure,
		MaxAge:   int(ttl.Seconds()),
		Expires:  time.Now().UTC().Add(ttl),
	})
}

func (s *server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.cfg.SessionCookieSecure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	})
}

type fixedWindowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	byIP   map[string]fixedWindowEntry
}

type fixedWindowEntry struct {
	start time.Time
	count int
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	if limit <= 0 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &fixedWindowLimiter{
		limit:  limit,
		window: window,
		byIP:   make(map[string]fixedWindowEntry),
	}
}

func (l *fixedWindowLimiter) Allow(ip string, now time.Time) bool {
	key := strings.TrimSpace(ip)
	if key == "" {
		key = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.byIP[key]
	if entry.start.IsZero() || now.Sub(entry.start) >= l.window {
		entry = fixedWindowEntry{start: now, count: 0}
	}
	if entry.count >= l.limit {
		l.byIP[key] = entry
		return false
	}
	entry.count++
	l.byIP[key] = entry
	return true
}

func (s *server) handleDeploymentRoutes(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/deployments/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) < 1 || strings.TrimSpace(parts[0]) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	deploymentID := strings.TrimSpace(parts[0])
	if len(parts) == 1 && s.handleDeploymentV2Detail(w, r, deploymentID) {
		return
	}
	if len(parts) == 2 && parts[1] == "redeploy" {
		s.handleDeploymentRedeployV2(w, r, deploymentID)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" {
		s.handleDeploymentCancelV2(w, r, deploymentID)
		return
	}
	if len(parts) == 2 && parts[1] == "rollback" {
		s.handleDeploymentRollbackV2(w, r, deploymentID)
		return
	}
	switch {
	case len(parts) == 2 && parts[1] == "logs":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleDeploymentLogsTail(w, r, deploymentID)
	case len(parts) == 3 && parts[1] == "logs" && parts[2] == "live":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleDeploymentLogsLive(w, r, deploymentID)
	case len(parts) == 2 && parts[1] == "steps":
		s.handleDeploymentSteps(w, r, deploymentID)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
	}
}

func (s *server) handleDeploymentLogsTail(w http.ResponseWriter, r *http.Request, deploymentID string) {
	deployment, err := s.store.GetDeploymentByID(r.Context(), deploymentID)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "deployment_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "deployment_lookup_failed"})
		return
	}
	logsPath := strings.TrimSpace(deployment.LogsPath)
	if logsPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "deployment_log_not_available"})
		return
	}
	tailBytes := parseQueryInt(r, "tail_bytes", logsapi.DefaultTailBytes)
	if tailBytes > logsapi.MaxTailBytes {
		tailBytes = logsapi.MaxTailBytes
	}
	tailLines := parseQueryInt(r, "tail_lines", 0)
	content, eof, err := logsapi.TailFileWithEOF(logsPath, tailBytes, tailLines)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "deployment_log_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "read_deployment_log_failed"})
		return
	}
	// JSON body carries eof when reverse proxies strip custom response headers (common in dev).
	eofMeta := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("eof_meta")), "true") ||
		strings.TrimSpace(r.URL.Query().Get("eof_meta")) == "1"
	if eofMeta {
		writeJSON(w, http.StatusOK, map[string]any{
			"eof":  eof,
			"text": string(content),
		})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Log-EOF-Offset", strconv.FormatInt(eof, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// checkWSOrigin rejects a cross-origin WebSocket upgrade unless the Origin is
// the configured platform domain (or a subdomain of it) or loopback. An
// absent Origin header is allowed: browsers always send Origin on a
// cross-origin upgrade, so its absence means a non-browser client (curl,
// internal tooling), not a spoofed browser request.
func (s *server) checkWSOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	if reqHost := strings.ToLower(strings.TrimSpace(r.Host)); reqHost != "" {
		if h, _, splitErr := net.SplitHostPort(reqHost); splitErr == nil {
			reqHost = h
		}
		if host == reqHost {
			return true
		}
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	if base := strings.TrimSpace(s.cfg.PlatformDomainBase); base != "" {
		if host == base || strings.HasSuffix(host, "."+base) {
			return true
		}
	}
	return false
}

func errorsIsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func branchFromRef(ref string) (string, bool) {
	const prefix = "refs/heads/"
	if !strings.HasPrefix(ref, prefix) {
		return "", false
	}
	branch := strings.TrimSpace(strings.TrimPrefix(ref, prefix))
	if branch == "" {
		return "", false
	}
	return branch, true
}

func isZeroSHA(raw string) bool {
	sha := strings.TrimSpace(raw)
	if sha == "" {
		return true
	}
	for _, ch := range sha {
		if ch != '0' {
			return false
		}
	}
	return true
}

func normalizeRoutePath(raw string) string {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "/hooks/github"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(payload)
}

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UTC().UnixNano())
	}
	return "req-" + hex.EncodeToString(buf)
}

func flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func cfgBoolDefault(envKey string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return def
	}
	val, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return val
}

func cfgIntDefault(envKey string, def int) int {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return def
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return val
}

func parseQueryInt(r *http.Request, key string, def int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return val
}

func requestIP(r *http.Request) string {
	directIP := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(directIP); err == nil {
		directIP = strings.TrimSpace(host)
	}
	peer := net.ParseIP(directIP)
	// Trust forwarding metadata only from the loopback proxy in front of the
	// management API. Direct clients cannot choose the CIDR used for automatic
	// database access by supplying X-Forwarded-For themselves.
	if peer != nil && peer.IsLoopback() {
		forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
		parts := strings.Split(forwarded, ",")
		for _, candidate := range parts {
			trimmed := strings.TrimSpace(candidate)
			if parsed := net.ParseIP(trimmed); parsed != nil && !parsed.IsLoopback() && !parsed.IsUnspecified() {
				return parsed.String()
			}
		}
	}
	return directIP
}

func requestCIDR(r *http.Request) string {
	clientIP := net.ParseIP(requestIP(r))
	if clientIP == nil || clientIP.IsLoopback() || clientIP.IsUnspecified() {
		return ""
	}
	if clientIP.To4() != nil {
		return clientIP.String() + "/32"
	}
	return clientIP.String() + "/128"
}
