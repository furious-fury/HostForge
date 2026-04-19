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
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/database"
	"github.com/hostforge/hostforge/internal/logging"
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
	if cfg.WebhookMaxBodyBytes <= 0 {
		fmt.Fprintln(os.Stderr, "error: webhook max body bytes must be > 0")
		return 2
	}
	if err := services.ValidateRuntimeConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: runtime config: %v\n", err)
		return 2
	}

	for _, d := range []string{cfg.DataDir, cfg.WorktreesDir(), cfg.BuildsDir()} {
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
	handler := &server{
		log:   log,
		cfg:   cfg,
		store: store,
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.WebhookBasePath, handler.handleGitHubWebhook)
	if cfg.WebhookSecret == "" {
		log.Warn("webhook shared secret is not configured; endpoint is network-reachable if exposed", "path", cfg.WebhookBasePath)
	}

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
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
	log   *slog.Logger
	cfg   *config.Config
	store *repository.Store
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
	if s.cfg.WebhookSecret != "" {
		token := strings.TrimSpace(r.Header.Get("X-HostForge-Token"))
		if token != s.cfg.WebhookSecret {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"status":     "error",
				"request_id": requestID,
				"error":      "unauthorized",
			})
			return
		}
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

	var payload githubPushPayload
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&payload); err != nil {
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

func writeJSON(w http.ResponseWriter, status int, payload map[string]string) {
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
