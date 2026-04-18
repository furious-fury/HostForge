package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds HostForge paths and runtime options for the CLI and server.
type Config struct {
	// DataDir is the root for worktrees, build outputs, and (later) SQLite.
	DataDir string
	// ListenAddr is the bind address for the HTTP API (Phase 4+). Unused by the Phase 0 CLI.
	ListenAddr string
}

// DataDirEnv is the environment variable overriding the default data directory.
const DataDirEnv = "HOSTFORGE_DATA_DIR"

// ListenEnv sets the API listen address (default ":8080"). Used by cmd/server in later phases.
const ListenEnv = "HOSTFORGE_LISTEN"

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
	return &Config{DataDir: abs, ListenAddr: listen}, nil
}

// WorktreesDir returns the directory for git worktrees.
func (c *Config) WorktreesDir() string {
	return filepath.Join(c.DataDir, "worktrees")
}

// BuildsDir returns the directory for nixpacks build outputs.
func (c *Config) BuildsDir() string {
	return filepath.Join(c.DataDir, "builds")
}
