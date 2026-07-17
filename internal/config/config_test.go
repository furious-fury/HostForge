package config

import (
	"path/filepath"
	"testing"
)

func TestCaddyControlPlanePathDefaultsBesideGeneratedRoutes(t *testing.T) {
	dataDir := t.TempDir()
	generated := filepath.Join(dataDir, "managed", "routes.caddy")
	t.Setenv(CaddyGeneratedPathEnv, generated)
	t.Setenv(CaddyControlPlanePathEnv, "")

	cfg, err := Load(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(generated), "control-plane.caddy")
	if cfg.CaddyControlPlanePath != want {
		t.Fatalf("control-plane path = %q, want %q", cfg.CaddyControlPlanePath, want)
	}
}

func TestCaddyControlPlanePathAllowsExplicitOverride(t *testing.T) {
	dataDir := t.TempDir()
	controlPlane := filepath.Join(dataDir, "caddy", "dashboard.caddy")
	t.Setenv(CaddyControlPlanePathEnv, controlPlane)

	cfg, err := Load(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CaddyControlPlanePath != controlPlane {
		t.Fatalf("control-plane path = %q, want %q", cfg.CaddyControlPlanePath, controlPlane)
	}
}

func TestDatabaseOperationSafetyDefaultsAndBounds(t *testing.T) {
	t.Setenv(DatabaseOperationConcurrencyEnv, "")
	t.Setenv(DatabaseTransferMaxPerHourEnv, "")
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseOperationConcurrency != 1 || cfg.DatabaseTransferMaxPerHour != 60 {
		t.Fatalf("unexpected database safety defaults: concurrency=%d transfers=%d", cfg.DatabaseOperationConcurrency, cfg.DatabaseTransferMaxPerHour)
	}

	t.Setenv(DatabaseOperationConcurrencyEnv, "9")
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("out-of-range database operation concurrency was accepted")
	}
	t.Setenv(DatabaseOperationConcurrencyEnv, "1")
	t.Setenv(DatabaseTransferMaxPerHourEnv, "0")
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("non-positive database transfer limit was accepted")
	}
}
