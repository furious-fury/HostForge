// Package config loads HostForge paths and runtime defaults from flags and environment.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds HostForge paths and runtime options for the CLI and server.
type Config struct {
	// DataDir is the root for worktrees, build outputs, and the SQLite DB (hostforge.db).
	DataDir string
	// ListenAddr is the bind address for the HTTP API (Phase 4+). Unused by the Phase 0 CLI.
	ListenAddr string
	// HostPort controls the published host port for deploy runs.
	// 0 = ephemeral, -1 = choose from configured range.
	HostPort int
	// PortStart and PortEnd define the inclusive host port range used when HostPort == -1.
	PortStart int
	PortEnd   int
	// ContainerPort is the container port the app listens on.
	ContainerPort int
	// CaddyBin is the executable used for validate/reload operations.
	CaddyBin string
	// CaddyGeneratedPath is the HostForge-managed generated Caddy config/snippet path.
	CaddyGeneratedPath string
	// CaddyRootConfig is the root Caddy config path for validate/reload.
	CaddyRootConfig string
	// SyncCaddy enables automatic caddy sync after successful deploy.
	SyncCaddy bool
	// HealthPath is the HTTP path used to probe new containers before cutover.
	HealthPath string
	// HealthTimeoutMS is per-request health probe timeout in milliseconds.
	HealthTimeoutMS int
	// HealthRetries is the number of health probe attempts before failing deploy.
	HealthRetries int
	// HealthIntervalMS is the delay between health probe attempts in milliseconds.
	HealthIntervalMS int
	// HealthExpectedMin is the minimum accepted HTTP status code for health checks.
	HealthExpectedMin int
	// HealthExpectedMax is the maximum accepted HTTP status code for health checks.
	HealthExpectedMax int
	// WebhookBasePath is the HTTP path for receiving provider webhook POSTs.
	WebhookBasePath string
	// WebhookMaxBodyBytes is the max accepted request body size for webhook payloads.
	WebhookMaxBodyBytes int
	// WebhookAsync controls whether webhook deploys are accepted (202) and run in background.
	WebhookAsync bool
	// WebhookSecret is an optional shared secret checked on webhook requests.
	WebhookSecret string
	// APIToken is the static bearer token used for management API authentication.
	APIToken string
	// SessionSecret is the HMAC key used to sign UI session cookies.
	SessionSecret string
	// SessionCookieName is the cookie name used for UI sessions.
	SessionCookieName string
	// SessionTTLMinutes controls signed session validity duration in minutes.
	SessionTTLMinutes int
	// SessionCookieSecure toggles the Secure flag on session cookies.
	SessionCookieSecure bool
	// WebhookRateLimitPerMinute is a basic per-IP webhook request ceiling.
	WebhookRateLimitPerMinute int
	// LogsDirPath overrides where build logs are written (default: <data-dir>/logs).
	LogsDirPath string
	// DNSServerIPv4 is an explicit public IPv4 for DNS A record suggestions (overrides auto-detect).
	DNSServerIPv4 string
	// DNSServerIPv6 is an explicit public IPv6 for AAAA suggestions (overrides auto-detect).
	DNSServerIPv6 string
	// DNSDetectURL is an HTTPS endpoint returning the server's public IPv4 as plain text.
	DNSDetectURL string
	// DNSDetectIPv6URL is an optional HTTPS endpoint returning public IPv6 as plain text.
	DNSDetectIPv6URL string
	// DNSDetectTimeoutMS bounds outbound IP discovery HTTP calls.
	DNSDetectTimeoutMS int
	// DomainSyncAfterMutate runs Caddy sync after domain add/edit/delete when root config is set.
	DomainSyncAfterMutate bool
	// CaddyCertPollIntervalSec controls optional background cert observation (0 = disabled).
	CaddyCertPollIntervalSec int
	// CaddyAdminURL is the Caddy admin API base (read-only GETs), e.g. http://127.0.0.1:2019.
	CaddyAdminURL string
	// CaddyStorageRoot is optional on-disk Caddy data dir (e.g. ~/.local/share/caddy) for leaf cert scans.
	CaddyStorageRoot string
}

// DataDirEnv is the environment variable overriding the default data directory.
const DataDirEnv = "HOSTFORGE_DATA_DIR"

// ListenEnv sets the API listen address (default ":8080"). Used by cmd/server in later phases.
const ListenEnv = "HOSTFORGE_LISTEN"

// HostPortEnv sets the exact host port used for deploy runs.
// 0 means ephemeral, -1 (default) means "pick from range".
const HostPortEnv = "HOSTFORGE_HOST_PORT"

// PortStartEnv and PortEndEnv define the inclusive host port range.
const (
	PortStartEnv = "HOSTFORGE_PORT_START"
	PortEndEnv   = "HOSTFORGE_PORT_END"
)

// ContainerPortEnv sets the app port inside the container.
const ContainerPortEnv = "HOSTFORGE_CONTAINER_PORT"

const (
	// CaddyBinEnv overrides the caddy executable path.
	CaddyBinEnv = "HOSTFORGE_CADDY_BIN"
	// CaddyGeneratedPathEnv sets where HostForge writes generated Caddy config.
	CaddyGeneratedPathEnv = "HOSTFORGE_CADDY_GENERATED_PATH"
	// CaddyRootConfigEnv sets the root Caddy config used for validate/reload.
	CaddyRootConfigEnv = "HOSTFORGE_CADDY_ROOT_CONFIG"
	// SyncCaddyEnv enables post-deploy Caddy sync when set to true.
	SyncCaddyEnv = "HOSTFORGE_SYNC_CADDY"
	// HealthPathEnv configures the HTTP path used for container readiness probes.
	HealthPathEnv = "HOSTFORGE_HEALTH_PATH"
	// HealthTimeoutMSEnv sets per-request health check timeout in milliseconds.
	HealthTimeoutMSEnv = "HOSTFORGE_HEALTH_TIMEOUT_MS"
	// HealthRetriesEnv sets how many health probe attempts deploy will perform.
	HealthRetriesEnv = "HOSTFORGE_HEALTH_RETRIES"
	// HealthIntervalMSEnv sets delay between health probe attempts in milliseconds.
	HealthIntervalMSEnv = "HOSTFORGE_HEALTH_INTERVAL_MS"
	// HealthExpectedMinEnv sets the minimum accepted health status code.
	HealthExpectedMinEnv = "HOSTFORGE_HEALTH_EXPECTED_MIN"
	// HealthExpectedMaxEnv sets the maximum accepted health status code.
	HealthExpectedMaxEnv = "HOSTFORGE_HEALTH_EXPECTED_MAX"
	// WebhookBasePathEnv configures the server route path for webhook POST requests.
	WebhookBasePathEnv = "HOSTFORGE_WEBHOOK_BASE_PATH"
	// WebhookMaxBodyBytesEnv sets a max webhook payload size in bytes.
	WebhookMaxBodyBytesEnv = "HOSTFORGE_WEBHOOK_MAX_BODY_BYTES"
	// WebhookAsyncEnv enables async webhook acceptance mode (HTTP 202 + background deploy).
	WebhookAsyncEnv = "HOSTFORGE_WEBHOOK_ASYNC"
	// WebhookSecretEnv sets an optional shared-secret token for webhook requests.
	WebhookSecretEnv = "HOSTFORGE_WEBHOOK_SECRET"
	// APITokenEnv is the static management API bearer token.
	APITokenEnv = "HOSTFORGE_API_TOKEN"
	// SessionSecretEnv is the HMAC key for signing UI session cookies.
	SessionSecretEnv = "HOSTFORGE_SESSION_SECRET"
	// SessionCookieNameEnv overrides the session cookie name used by the server.
	SessionCookieNameEnv = "HOSTFORGE_SESSION_COOKIE_NAME"
	// SessionTTLMinutesEnv sets UI session lifetime in minutes.
	SessionTTLMinutesEnv = "HOSTFORGE_SESSION_TTL_MINUTES"
	// SessionCookieSecureEnv toggles the Secure flag on UI session cookies.
	SessionCookieSecureEnv = "HOSTFORGE_SESSION_COOKIE_SECURE"
	// WebhookRateLimitPerMinuteEnv sets a simple per-IP webhook rate cap.
	WebhookRateLimitPerMinuteEnv = "HOSTFORGE_WEBHOOK_RATE_LIMIT_PER_MINUTE"
	// LogsDirEnv overrides the default logs directory under data dir.
	LogsDirEnv = "HOSTFORGE_LOGS_DIR"
	// DNSServerIPv4Env sets a fixed IPv4 for DNS guidance (skips auto-detect when set).
	DNSServerIPv4Env = "HOSTFORGE_DNS_SERVER_IPV4"
	// DNSServerIPv6Env sets a fixed IPv6 for DNS guidance (optional).
	DNSServerIPv6Env = "HOSTFORGE_DNS_SERVER_IPV6"
	// DNSDetectURLEnv overrides the default IPv4 discovery URL (plain-text IP response).
	DNSDetectURLEnv = "HOSTFORGE_DNS_DETECT_URL"
	// DNSDetectIPv6URLEnv sets an optional IPv6 discovery URL (plain-text IP response).
	DNSDetectIPv6URLEnv = "HOSTFORGE_DNS_DETECT_IPV6_URL"
	// DNSDetectTimeoutMSEnv bounds outbound IP discovery HTTP calls.
	DNSDetectTimeoutMSEnv = "HOSTFORGE_DNS_DETECT_TIMEOUT_MS"
	// DomainSyncAfterMutateEnv toggles Caddy sync after domain CRUD API calls when root config exists.
	DomainSyncAfterMutateEnv = "HOSTFORGE_DOMAIN_SYNC_AFTER_MUTATE"
	// CaddyCertPollIntervalSecEnv sets seconds between optional cert polls (0 disables).
	CaddyCertPollIntervalSecEnv = "HOSTFORGE_CADDY_CERT_POLL_INTERVAL_SEC"
	// CaddyAdminURLEnv overrides the Caddy admin API base URL for read-only probes.
	CaddyAdminURLEnv = "HOSTFORGE_CADDY_ADMIN"
	// CaddyStorageRootEnv points at Caddy's on-disk storage root for certificate file scans.
	CaddyStorageRootEnv = "HOSTFORGE_CADDY_STORAGE_ROOT"
	// EnvEncryptionKeyEnv is an optional base64-encoded 32-byte AES-256 key used to encrypt
	// per-project environment variable values at rest (see README). When unset, env CRUD
	// API returns 503 and deploy skips injecting project env (unless ciphertext rows exist).
	EnvEncryptionKeyEnv = "HOSTFORGE_ENV_ENCRYPTION_KEY"
)

// DefaultDataDir returns the default data directory (./.hostforge).
func DefaultDataDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, ".hostforge"), nil
}

// Load resolves DataDir from flag override, then env, then default.
func Load(dataDirFlag string) (*Config, error) {
	var dir string
	switch {
	case dataDirFlag != "":
		dir = dataDirFlag
	case os.Getenv(DataDirEnv) != "":
		dir = os.Getenv(DataDirEnv)
	default:
		var err error
		dir, err = DefaultDataDir()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve data dir: %w", err)
	}
	listen := os.Getenv(ListenEnv)
	if listen == "" {
		listen = ":8080"
	}
	hostPort, err := envInt(HostPortEnv, -1)
	if err != nil {
		return nil, err
	}
	portStart, err := envInt(PortStartEnv, 20000)
	if err != nil {
		return nil, err
	}
	portEnd, err := envInt(PortEndEnv, 21000)
	if err != nil {
		return nil, err
	}
	containerPort, err := envInt(ContainerPortEnv, 3000)
	if err != nil {
		return nil, err
	}
	caddyBin := strings.TrimSpace(os.Getenv(CaddyBinEnv))
	if caddyBin == "" {
		caddyBin = "caddy"
	}
	caddyGeneratedPath := strings.TrimSpace(os.Getenv(CaddyGeneratedPathEnv))
	if caddyGeneratedPath == "" {
		caddyGeneratedPath = filepath.Join(abs, "caddy", "hostforge.caddy")
	}
	caddyRootConfig := strings.TrimSpace(os.Getenv(CaddyRootConfigEnv))
	syncCaddy, err := envBool(SyncCaddyEnv, false)
	if err != nil {
		return nil, err
	}
	healthPath := strings.TrimSpace(os.Getenv(HealthPathEnv))
	if healthPath == "" {
		healthPath = "/"
	}
	healthTimeoutMS, err := envInt(HealthTimeoutMSEnv, 3000)
	if err != nil {
		return nil, err
	}
	healthRetries, err := envInt(HealthRetriesEnv, 10)
	if err != nil {
		return nil, err
	}
	healthIntervalMS, err := envInt(HealthIntervalMSEnv, 1000)
	if err != nil {
		return nil, err
	}
	healthExpectedMin, err := envInt(HealthExpectedMinEnv, 200)
	if err != nil {
		return nil, err
	}
	healthExpectedMax, err := envInt(HealthExpectedMaxEnv, 399)
	if err != nil {
		return nil, err
	}
	webhookBasePath := strings.TrimSpace(os.Getenv(WebhookBasePathEnv))
	if webhookBasePath == "" {
		webhookBasePath = "/hooks/github"
	}
	webhookMaxBodyBytes, err := envInt(WebhookMaxBodyBytesEnv, 1_048_576)
	if err != nil {
		return nil, err
	}
	webhookAsync, err := envBool(WebhookAsyncEnv, false)
	if err != nil {
		return nil, err
	}
	webhookSecret := strings.TrimSpace(os.Getenv(WebhookSecretEnv))
	apiToken := strings.TrimSpace(os.Getenv(APITokenEnv))
	sessionSecret := strings.TrimSpace(os.Getenv(SessionSecretEnv))
	sessionCookieName := strings.TrimSpace(os.Getenv(SessionCookieNameEnv))
	if sessionCookieName == "" {
		sessionCookieName = "hostforge_session"
	}
	sessionTTLMinutes, err := envInt(SessionTTLMinutesEnv, 720)
	if err != nil {
		return nil, err
	}
	sessionCookieSecure, err := envBool(SessionCookieSecureEnv, false)
	if err != nil {
		return nil, err
	}
	webhookRateLimitPerMinute, err := envInt(WebhookRateLimitPerMinuteEnv, 60)
	if err != nil {
		return nil, err
	}
	logsDirPath := strings.TrimSpace(os.Getenv(LogsDirEnv))
	dnsServerIPv4 := strings.TrimSpace(os.Getenv(DNSServerIPv4Env))
	dnsServerIPv6 := strings.TrimSpace(os.Getenv(DNSServerIPv6Env))
	dnsDetectURL := strings.TrimSpace(os.Getenv(DNSDetectURLEnv))
	if dnsDetectURL == "" {
		dnsDetectURL = "https://api.ipify.org"
	}
	dnsDetectIPv6URL := strings.TrimSpace(os.Getenv(DNSDetectIPv6URLEnv))
	if dnsDetectIPv6URL == "" {
		dnsDetectIPv6URL = "https://api64.ipify.org"
	}
	dnsDetectTimeoutMS, err := envInt(DNSDetectTimeoutMSEnv, 2500)
	if err != nil {
		return nil, err
	}
	domainSyncAfterMutate, err := envBool(DomainSyncAfterMutateEnv, true)
	if err != nil {
		return nil, err
	}
	caddyCertPollIntervalSec, err := envInt(CaddyCertPollIntervalSecEnv, 0)
	if err != nil {
		return nil, err
	}
	caddyAdminURL := strings.TrimSpace(os.Getenv(CaddyAdminURLEnv))
	if caddyAdminURL == "" {
		caddyAdminURL = "http://127.0.0.1:2019"
	}
	caddyStorageRoot := expandUserPath(strings.TrimSpace(os.Getenv(CaddyStorageRootEnv)))
	return &Config{
		DataDir:                   abs,
		ListenAddr:                listen,
		HostPort:                  hostPort,
		PortStart:                 portStart,
		PortEnd:                   portEnd,
		ContainerPort:             containerPort,
		CaddyBin:                  caddyBin,
		CaddyGeneratedPath:        caddyGeneratedPath,
		CaddyRootConfig:           caddyRootConfig,
		SyncCaddy:                 syncCaddy,
		HealthPath:                healthPath,
		HealthTimeoutMS:           healthTimeoutMS,
		HealthRetries:             healthRetries,
		HealthIntervalMS:          healthIntervalMS,
		HealthExpectedMin:         healthExpectedMin,
		HealthExpectedMax:         healthExpectedMax,
		WebhookBasePath:           webhookBasePath,
		WebhookMaxBodyBytes:       webhookMaxBodyBytes,
		WebhookAsync:              webhookAsync,
		WebhookSecret:             webhookSecret,
		APIToken:                  apiToken,
		SessionSecret:             sessionSecret,
		SessionCookieName:         sessionCookieName,
		SessionTTLMinutes:         sessionTTLMinutes,
		SessionCookieSecure:       sessionCookieSecure,
		WebhookRateLimitPerMinute: webhookRateLimitPerMinute,
		LogsDirPath:               logsDirPath,
		DNSServerIPv4:             dnsServerIPv4,
		DNSServerIPv6:             dnsServerIPv6,
		DNSDetectURL:              dnsDetectURL,
		DNSDetectIPv6URL:          dnsDetectIPv6URL,
		DNSDetectTimeoutMS:        dnsDetectTimeoutMS,
		DomainSyncAfterMutate:     domainSyncAfterMutate,
		CaddyCertPollIntervalSec: caddyCertPollIntervalSec,
		CaddyAdminURL:            caddyAdminURL,
		CaddyStorageRoot:         caddyStorageRoot,
	}, nil
}

// expandUserPath replaces a leading "~" or "~/" with the current user's home directory.
func expandUserPath(p string) string {
	if p == "" {
		return ""
	}
	if p == "~" {
		if h, err := os.UserHomeDir(); err == nil {
			return h
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, strings.TrimPrefix(p, "~/"))
		}
	}
	return filepath.Clean(p)
}

// WorktreesDir returns the directory for git worktrees.
func (c *Config) WorktreesDir() string {
	return filepath.Join(c.DataDir, "worktrees")
}

// BuildsDir returns the directory for nixpacks build outputs.
func (c *Config) BuildsDir() string {
	return filepath.Join(c.DataDir, "builds")
}

// DBPath returns the SQLite control-plane database path.
func (c *Config) DBPath() string {
	return filepath.Join(c.DataDir, "hostforge.db")
}

// LogsDir returns the directory for build/deployment log files.
func (c *Config) LogsDir() string {
	if strings.TrimSpace(c.LogsDirPath) != "" {
		return c.LogsDirPath
	}
	return filepath.Join(c.DataDir, "logs")
}

func envInt(key string, def int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return def, nil
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return val, nil
}

func envBool(key string, def bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	val, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return val, nil
}

// RuntimeDefaults returns runtime port settings resolved from env with built-in defaults.
func RuntimeDefaults() (hostPort, portStart, portEnd, containerPort int, err error) {
	hostPort, err = envInt(HostPortEnv, -1)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	portStart, err = envInt(PortStartEnv, 20000)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	portEnd, err = envInt(PortEndEnv, 21000)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	containerPort, err = envInt(ContainerPortEnv, 3000)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return hostPort, portStart, portEnd, containerPort, nil
}

// HealthDefaults returns deploy health-check settings resolved from env with defaults.
func HealthDefaults() (path string, timeoutMS, retries, intervalMS, expectedMin, expectedMax int, err error) {
	path = strings.TrimSpace(os.Getenv(HealthPathEnv))
	if path == "" {
		path = "/"
	}
	timeoutMS, err = envInt(HealthTimeoutMSEnv, 3000)
	if err != nil {
		return "", 0, 0, 0, 0, 0, err
	}
	retries, err = envInt(HealthRetriesEnv, 10)
	if err != nil {
		return "", 0, 0, 0, 0, 0, err
	}
	intervalMS, err = envInt(HealthIntervalMSEnv, 1000)
	if err != nil {
		return "", 0, 0, 0, 0, 0, err
	}
	expectedMin, err = envInt(HealthExpectedMinEnv, 200)
	if err != nil {
		return "", 0, 0, 0, 0, 0, err
	}
	expectedMax, err = envInt(HealthExpectedMaxEnv, 399)
	if err != nil {
		return "", 0, 0, 0, 0, 0, err
	}
	return path, timeoutMS, retries, intervalMS, expectedMin, expectedMax, nil
}
