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
