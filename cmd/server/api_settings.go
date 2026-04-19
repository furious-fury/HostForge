package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/auth"
	"github.com/hostforge/hostforge/internal/caddy"
	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/dnsops"
	"github.com/hostforge/hostforge/internal/services"
	"github.com/hostforge/hostforge/internal/sysstatus"
	"github.com/hostforge/hostforge/internal/version"
)

func (s *server) handleSettingsRoutes(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimSuffix(r.URL.Path, "/")
	if p == "" {
		p = r.URL.Path
	}
	if p == "/api/settings" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleSettingsGet(w, r)
		return
	}
	if strings.HasPrefix(p, "/api/settings/actions/") {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		action := strings.TrimSpace(strings.TrimPrefix(p, "/api/settings/actions/"))
		switch action {
		case "caddy-validate":
			s.handleSettingsActionCaddyValidate(w, r)
		case "caddy-sync":
			s.handleSettingsActionCaddySync(w, r)
		case "refresh-status":
			s.handleSettingsActionRefreshStatus(w, r)
		case "detect-public-ipv4":
			s.handleSettingsActionDetectPublicIPv4(w, r)
		default:
			http.NotFound(w, r)
		}
		return
	}
	http.NotFound(w, r)
}

func (s *server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	payload := s.buildSettingsPayload(r)
	writeJSON(w, http.StatusOK, payload)
}

func (s *server) buildSettingsPayload(r *http.Request) map[string]any {
	cfg := s.cfg
	now := time.Now().UTC()
	uptime := int64(0)
	if !serverStartedAt.IsZero() {
		uptime = int64(now.Sub(serverStartedAt).Seconds())
	}

	scheme, _ := s.authenticateRequest(r)
	authBlock := map[string]any{
		"scheme": scheme,
	}
	if scheme == "session" {
		cookie, err := r.Cookie(cfg.SessionCookieName)
		if err == nil && strings.TrimSpace(cookie.Value) != "" {
			if claims, err := auth.VerifySignedSession(cfg.SessionSecret, cookie.Value, now); err == nil {
				authBlock["expires_at"] = time.Unix(claims.Expires, 0).UTC().Format(time.RFC3339)
				authBlock["subject"] = claims.Subject
			}
		}
	}

	dctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	ipv4, v4src, v4warn := dnsops.ResolveExpectedIPv4(dctx, cfg)

	dbPath := cfg.DBPath()
	dbSize := int64(-1)
	if st, err := os.Stat(dbPath); err == nil {
		dbSize = st.Size()
	}

	logsDir := cfg.LogsDir()
	logsSize := int64(-1)
	if sz, err := dirSizeBytes(logsDir); err == nil {
		logsSize = sz
	}

	build := map[string]any{
		"version":         version.String(),
		"version_display": version.Display(),
		"commit":          strings.TrimSpace(version.Commit),
		"build_time":      strings.TrimSpace(version.BuildTime),
		"go_version":      runtime.Version(),
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
		"pid":             os.Getpid(),
		"started_at":      serverStartedAt.UTC().Format(time.RFC3339),
		"uptime_seconds":  uptime,
	}

	paths := map[string]any{
		"data_dir":             cfg.DataDir,
		"data_dir_env":         config.DataDirEnv,
		"logs_dir":             logsDir,
		"logs_dir_env":         config.LogsDirEnv,
		"db_path":              dbPath,
		"db_size_bytes":        dbSize,
		"logs_dir_size_bytes":  logsSize,
	}

	network := map[string]any{
		"listen":            cfg.ListenAddr,
		"listen_env":        config.ListenEnv,
		"host_port":         cfg.HostPort,
		"host_port_env":     config.HostPortEnv,
		"port_start":        cfg.PortStart,
		"port_start_env":    config.PortStartEnv,
		"port_end":          cfg.PortEnd,
		"port_end_env":      config.PortEndEnv,
		"container_port":    cfg.ContainerPort,
		"container_port_env": config.ContainerPortEnv,
	}

	health := map[string]any{
		"path":                 cfg.HealthPath,
		"path_env":            config.HealthPathEnv,
		"timeout_ms":          cfg.HealthTimeoutMS,
		"timeout_ms_env":      config.HealthTimeoutMSEnv,
		"retries":             cfg.HealthRetries,
		"retries_env":         config.HealthRetriesEnv,
		"interval_ms":         cfg.HealthIntervalMS,
		"interval_ms_env":     config.HealthIntervalMSEnv,
		"expected_min":        cfg.HealthExpectedMin,
		"expected_min_env":    config.HealthExpectedMinEnv,
		"expected_max":        cfg.HealthExpectedMax,
		"expected_max_env":    config.HealthExpectedMaxEnv,
	}

	caddyBlock := map[string]any{
		"bin":                        cfg.CaddyBin,
		"bin_env":                    config.CaddyBinEnv,
		"generated_path":             cfg.CaddyGeneratedPath,
		"generated_path_env":         config.CaddyGeneratedPathEnv,
		"root_config":                cfg.CaddyRootConfig,
		"root_config_env":            config.CaddyRootConfigEnv,
		"sync_caddy":                 cfg.SyncCaddy,
		"sync_caddy_env":             config.SyncCaddyEnv,
		"domain_sync_after_mutate":   cfg.DomainSyncAfterMutate,
		"domain_sync_after_mutate_env": config.DomainSyncAfterMutateEnv,
		"cert_poll_interval_sec":     cfg.CaddyCertPollIntervalSec,
		"cert_poll_interval_sec_env": config.CaddyCertPollIntervalSecEnv,
		"admin_url":                  cfg.CaddyAdminURL,
		"admin_url_env":              config.CaddyAdminURLEnv,
		"storage_root":               cfg.CaddyStorageRoot,
		"storage_root_env":           config.CaddyStorageRootEnv,
	}

	webhooks := map[string]any{
		"base_path":                 cfg.WebhookBasePath,
		"base_path_env":             config.WebhookBasePathEnv,
		"max_body_bytes":            cfg.WebhookMaxBodyBytes,
		"max_body_bytes_env":        config.WebhookMaxBodyBytesEnv,
		"async":                     cfg.WebhookAsync,
		"async_env":                 config.WebhookAsyncEnv,
		"rate_limit_per_minute":     cfg.WebhookRateLimitPerMinute,
		"rate_limit_per_minute_env": config.WebhookRateLimitPerMinuteEnv,
		"secret_set":                strings.TrimSpace(cfg.WebhookSecret) != "",
		"secret_env":                config.WebhookSecretEnv,
	}

	dns := map[string]any{
		"server_ipv4":            cfg.DNSServerIPv4,
		"server_ipv4_env":      config.DNSServerIPv4Env,
		"server_ipv6":            cfg.DNSServerIPv6,
		"server_ipv6_env":      config.DNSServerIPv6Env,
		"detect_url":             cfg.DNSDetectURL,
		"detect_url_env":         config.DNSDetectURLEnv,
		"detect_ipv6_url":        cfg.DNSDetectIPv6URL,
		"detect_ipv6_url_env":    config.DNSDetectIPv6URLEnv,
		"detect_timeout_ms":      cfg.DNSDetectTimeoutMS,
		"detect_timeout_ms_env":  config.DNSDetectTimeoutMSEnv,
		"detected_ipv4":          ipv4,
		"detected_ipv4_source":   v4src,
		"detected_ipv4_warning":  v4warn,
	}

	session := map[string]any{
		"cookie_name":           cfg.SessionCookieName,
		"cookie_name_env":       config.SessionCookieNameEnv,
		"ttl_minutes":           cfg.SessionTTLMinutes,
		"ttl_minutes_env":       config.SessionTTLMinutesEnv,
		"cookie_secure":         cfg.SessionCookieSecure,
		"cookie_secure_env":     config.SessionCookieSecureEnv,
		"session_secret_set":  strings.TrimSpace(cfg.SessionSecret) != "",
		"session_secret_env":    config.SessionSecretEnv,
		"api_token_set":         strings.TrimSpace(cfg.APIToken) != "",
		"api_token_env":         config.APITokenEnv,
	}

	return map[string]any{
		"auth":     authBlock,
		"build":    build,
		"paths":    paths,
		"network":  network,
		"health":   health,
		"caddy":    caddyBlock,
		"webhooks": webhooks,
		"dns":      dns,
		"session":  session,
	}
}

func dirSizeBytes(root string) (int64, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return 0, fmt.Errorf("empty path")
	}
	var total int64
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return nil
		}
		if info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (s *server) handleSettingsActionCaddyValidate(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(s.cfg.CaddyRootConfig)
	if root == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":     false,
			"error":  "caddy_root_config_not_set",
			"detail": "Set " + config.CaddyRootConfigEnv + " to your root Caddyfile path.",
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	t0 := time.Now()
	stdout, stderr, err := caddy.ValidateRootCapture(ctx, s.cfg.CaddyBin, root)
	ms := time.Since(t0).Milliseconds()
	ok := err == nil
	resp := map[string]any{
		"ok":       ok,
		"stdout":   strings.TrimSpace(stdout),
		"stderr":   strings.TrimSpace(stderr),
		"took_ms":  ms,
		"root":     root,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleSettingsActionCaddySync(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(s.cfg.CaddyRootConfig)
	if root == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"attempted": false,
			"ok":        false,
			"error":     "caddy_root_config_not_set",
		})
		return
	}
	t0 := time.Now()
	err := services.SyncCaddyRoutes(r.Context(), s.requestLog(r), s.cfg, s.store)
	out := caddySyncOutcome{Attempted: true}
	if err != nil {
		out.OK = false
		out.Error = publicAPIError(err, "caddy_sync_failed")
		s.requestLog(r).Warn("settings caddy sync failed", "error", err, "duration_ms", time.Since(t0).Milliseconds())
		writeJSON(w, http.StatusOK, map[string]any{
			"caddy_sync": out,
			"duration_ms": time.Since(t0).Milliseconds(),
		})
		return
	}
	out.OK = true
	s.requestLog(r).Info("settings caddy sync complete", "duration_ms", time.Since(t0).Milliseconds())
	writeJSON(w, http.StatusOK, map[string]any{
		"caddy_sync":  out,
		"duration_ms": time.Since(t0).Milliseconds(),
	})
}

func (s *server) handleSettingsActionRefreshStatus(w http.ResponseWriter, r *http.Request) {
	resp := sysstatus.Gather(r.Context(), s.cfg)
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleSettingsActionDetectPublicIPv4(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	ip, src, warn := dnsops.ResolveExpectedIPv4(ctx, s.cfg)
	writeJSON(w, http.StatusOK, map[string]any{
		"ipv4":    ip,
		"source":  src,
		"warning": warn,
	})
}
