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
	return &Config{
		DataDir:            abs,
		ListenAddr:         listen,
		HostPort:           hostPort,
		PortStart:          portStart,
		PortEnd:            portEnd,
		ContainerPort:      containerPort,
		CaddyBin:           caddyBin,
		CaddyGeneratedPath: caddyGeneratedPath,
		CaddyRootConfig:    caddyRootConfig,
		SyncCaddy:          syncCaddy,
		HealthPath:         healthPath,
		HealthTimeoutMS:    healthTimeoutMS,
		HealthRetries:      healthRetries,
		HealthIntervalMS:   healthIntervalMS,
		HealthExpectedMin:  healthExpectedMin,
		HealthExpectedMax:  healthExpectedMax,
	}, nil
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
