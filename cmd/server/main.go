package main

import (
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
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hostforge/hostforge/internal/auth"
	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/database"
	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/logging"
	logsapi "github.com/hostforge/hostforge/internal/logs"
	"github.com/hostforge/hostforge/internal/models"
	"github.com/hostforge/hostforge/internal/repository"
	"github.com/hostforge/hostforge/internal/services"
)

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
	if cfg.WebhookRateLimitPerMinute <= 0 {
		fmt.Fprintln(os.Stderr, "error: webhook rate limit per minute must be > 0")
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
	db, err := database.OpenSQLite(ctx, cfg.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: sqlite: %v\n", err)
		return 1
	}
	defer db.Close()

	store := repository.New(db)
	services.StartCaddyCertPollLoop(log, cfg, store)
	webhookLimiter := newFixedWindowLimiter(cfg.WebhookRateLimitPerMinute, time.Minute)
	handler := &server{
		log:            log,
		cfg:            cfg,
		store:          store,
		webhookLimiter: webhookLimiter,
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.WebhookBasePath, handler.handleGitHubWebhook)
	mux.HandleFunc("/auth/session", handler.handleSessionRoutes)
	mux.HandleFunc("/api/system/status", handler.requireManagementAuth(handler.handleSystemStatus))
	mux.HandleFunc("/api/repositories/branches", handler.requireManagementAuth(handler.handleRepositoryBranches))
	mux.HandleFunc("/api/projects", handler.requireManagementAuth(handler.handleProjectsCollection))
	mux.HandleFunc("/api/projects/", handler.requireManagementAuth(handler.handleProjectRoutes))
	mux.HandleFunc("/api/deployments", handler.requireManagementAuth(handler.handleDeploymentsCollection))
	mux.HandleFunc("/api/deployments/", handler.requireManagementAuth(handler.handleDeploymentRoutes))
	registerStaticUIRoutes(mux, log)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       0,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
	}
	log.Info("hostforge server listening", "listen", cfg.ListenAddr, "webhook_path", cfg.WebhookBasePath, "webhook_async", cfg.WebhookAsync)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "error: server: %v\n", err)
		return 1
	}
	return 0
}

type server struct {
	log            *slog.Logger
	cfg            *config.Config
	store          *repository.Store
	webhookLimiter *fixedWindowLimiter
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
		requestID = newRequestID()
	}
	log := s.log.With("request_id", requestID)
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
	if eventType != "" && eventType != "push" {
		log.Info("ignoring unsupported webhook event", "event", eventType)
		writeJSON(w, http.StatusOK, map[string]string{
			"status":     "ignored",
			"request_id": requestID,
			"reason":     "unsupported_event",
		})
		return
	}

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
	if !auth.VerifyGitHubSignature(s.cfg.WebhookSecret, signature, body) {
		log.Warn("webhook rejected", "reason", "signature_mismatch", "remote_ip", remoteIP)
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"status":     "error",
			"request_id": requestID,
			"error":      "invalid_signature",
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

	project, err := findProjectByRepoAndBranch(r.Context(), s.store, repoURL, payload.Repository.CloneURL, branch)
	if err != nil {
		if errorsIsNoRows(err) {
			repoExists, lookupErr := repoExistsForAnyBranch(r.Context(), s.store, repoURL, payload.Repository.CloneURL)
			if lookupErr != nil {
				log.Error("repo existence lookup failed", "repo_url", repoURL, "error", lookupErr)
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"status":     "error",
					"request_id": requestID,
					"error":      "project_lookup_failed",
				})
				return
			}
			if repoExists {
				log.Info("ignoring push for non-matching branch", "repo_url", repoURL, "branch", branch)
				writeJSON(w, http.StatusOK, map[string]string{
					"status":     "ignored",
					"request_id": requestID,
					"reason":     "branch_mismatch",
				})
				return
			}
			writeJSON(w, http.StatusNotFound, map[string]string{
				"status":     "error",
				"request_id": requestID,
				"error":      "project_not_found_for_repo_branch",
			})
			return
		}
		log.Error("project lookup failed", "repo_url", repoURL, "branch", branch, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"status":     "error",
			"request_id": requestID,
			"error":      "project_lookup_failed",
		})
		return
	}
	if strings.TrimSpace(project.Branch) == "" {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":     "ignored",
			"request_id": requestID,
			"reason":     "project_branch_not_configured",
		})
		return
	}

	job, err := services.PrepareDeploy(r.Context(), s.cfg, s.store, services.DeployPrepareInput{
		Project:    project,
		RepoURL:    repoURL,
		Branch:     branch,
		CommitHash: strings.TrimSpace(payload.After),
	})
	if err != nil {
		log.Error("failed to accept deployment", "project_id", project.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"status":     "error",
			"request_id": requestID,
			"error":      "failed_to_accept_deployment",
		})
		return
	}

	deployLog := log.With("project_id", project.ID, "deployment_id", job.Deployment.ID, "repo_url", repoURL, "branch", branch)
	if s.cfg.WebhookAsync {
		go func(job services.DeployJob) {
			_, execErr := services.ExecuteDeploy(context.Background(), deployLog, s.cfg, s.store, job)
			if execErr != nil {
				deployLog.Error("async deployment failed", "error", execErr)
			}
		}(job)
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":        "accepted",
			"request_id":    requestID,
			"deployment_id": job.Deployment.ID,
			"mode":          "async",
		})
		return
	}

	result, err := services.ExecuteDeploy(r.Context(), deployLog, s.cfg, s.store, job)
	if err != nil {
		deployLog.Error("synchronous deployment failed", "error", err)
		writeJSON(w, http.StatusOK, map[string]string{
			"status":        "failed",
			"request_id":    requestID,
			"deployment_id": job.Deployment.ID,
			"error":         err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":        "success",
		"request_id":    requestID,
		"deployment_id": result.DeploymentID,
		"container_id":  result.ContainerID,
		"url":           result.URL,
		"mode":          "sync",
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
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || parts[1] != "logs" {
		http.NotFound(w, r)
		return
	}
	deploymentID := strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleDeploymentLogsTail(w, r, deploymentID)
		return
	}
	if len(parts) == 3 && parts[2] == "live" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleDeploymentLogsLive(w, r, deploymentID)
		return
	}
	http.NotFound(w, r)
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
	content, err := logsapi.TailFile(logsPath, tailBytes, tailLines)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "deployment_log_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "read_deployment_log_failed"})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

var logUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

func (s *server) handleDeploymentLogsLive(w http.ResponseWriter, r *http.Request, deploymentID string) {
	deployment, err := s.store.GetDeploymentByID(r.Context(), deploymentID)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "deployment_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "deployment_lookup_failed"})
		return
	}
	conn, err := logUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	source := strings.TrimSpace(r.URL.Query().Get("source"))
	if source == "" {
		source = defaultLogSource(deployment)
	}
	switch source {
	case "build":
		s.streamBuildLog(ctx, conn, deployment)
	case "container":
		s.streamContainerLog(ctx, conn, deploymentID)
	default:
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: unsupported source"))
	}
}

func (s *server) streamBuildLog(ctx context.Context, conn *websocket.Conn, deployment models.Deployment) {
	if strings.TrimSpace(deployment.LogsPath) == "" {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: deployment log path is empty"))
		return
	}
	initial, err := logsapi.TailFile(deployment.LogsPath, logsapi.DefaultTailBytes, 0)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: failed to tail deployment log"))
		return
	}
	if len(initial) > 0 {
		_ = conn.WriteMessage(websocket.TextMessage, initial)
	}
	if err := logsapi.FollowFile(ctx, deployment.LogsPath, 500*time.Millisecond, func(chunk []byte) error {
		if len(chunk) == 0 {
			return nil
		}
		return conn.WriteMessage(websocket.TextMessage, chunk)
	}); err != nil && !errors.Is(err, context.Canceled) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: build log stream ended"))
	}
}

func (s *server) streamContainerLog(ctx context.Context, conn *websocket.Conn, deploymentID string) {
	containerRec, err := s.store.GetContainerByDeploymentID(ctx, deploymentID)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: container record not found for deployment"))
		return
	}
	cli, err := docker.NewClient(ctx)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: docker unavailable"))
		return
	}
	defer cli.Close()
	writer := &websocketWriter{conn: conn}
	if err := docker.StreamContainerLogs(ctx, cli, containerRec.DockerContainerID, docker.LogStreamOptions{
		Follow:     true,
		Tail:       "200",
		ShowStdout: true,
		ShowStderr: true,
	}, writer); err != nil && !errors.Is(err, context.Canceled) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: container log stream ended"))
	}
}

type websocketWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *websocketWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	if err := w.conn.WriteMessage(websocket.TextMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func defaultLogSource(deployment models.Deployment) string {
	if deployment.Status == models.DeploymentSuccess {
		return "container"
	}
	return "build"
}

func findProjectByRepoAndBranch(ctx context.Context, store *repository.Store, canonicalRepoURL, rawRepoURL, branch string) (models.Project, error) {
	candidates := []string{strings.TrimSpace(canonicalRepoURL)}
	raw := strings.TrimSpace(rawRepoURL)
	if raw != "" && raw != canonicalRepoURL {
		candidates = append(candidates, raw)
	}

	var lastErr error
	for _, candidate := range candidates {
		project, err := store.GetProjectByRepoAndBranch(ctx, candidate, branch)
		if err == nil {
			return project, nil
		}
		lastErr = err
		if !errorsIsNoRows(err) {
			return models.Project{}, err
		}
	}
	if lastErr == nil {
		lastErr = sql.ErrNoRows
	}
	return models.Project{}, lastErr
}

func repoExistsForAnyBranch(ctx context.Context, store *repository.Store, canonicalRepoURL, rawRepoURL string) (bool, error) {
	candidates := []string{strings.TrimSpace(canonicalRepoURL)}
	raw := strings.TrimSpace(rawRepoURL)
	if raw != "" && raw != canonicalRepoURL {
		candidates = append(candidates, raw)
	}

	for _, candidate := range candidates {
		projects, err := store.ListProjectsByRepoURL(ctx, candidate)
		if err != nil {
			return false, err
		}
		if len(projects) > 0 {
			return true, nil
		}
	}
	return false, nil
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
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		for _, candidate := range parts {
			trimmed := strings.TrimSpace(candidate)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && strings.TrimSpace(host) != "" {
		return strings.TrimSpace(host)
	}
	return strings.TrimSpace(r.RemoteAddr)
}
