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

// App container resource limits (ADR-0002 §14.2) are fleet-wide, config-driven
// defaults. 0 is a valid, deliberate value meaning "disable this limit,"
// matching Docker's own semantics; only negative values are rejected.
func TestAppContainerResourceLimitDefaultsAndBounds(t *testing.T) {
	t.Setenv(AppContainerMemoryLimitBytesEnv, "")
	t.Setenv(AppContainerCPULimitMillisEnv, "")
	t.Setenv(AppContainerPidsLimitEnv, "")
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppContainerMemoryLimitBytes != 512*1024*1024 || cfg.AppContainerCPULimitMillis != 1000 || cfg.AppContainerPidsLimit != 512 {
		t.Fatalf("unexpected app container defaults: memory=%d cpu=%d pids=%d",
			cfg.AppContainerMemoryLimitBytes, cfg.AppContainerCPULimitMillis, cfg.AppContainerPidsLimit)
	}

	t.Setenv(AppContainerMemoryLimitBytesEnv, "0")
	t.Setenv(AppContainerCPULimitMillisEnv, "0")
	t.Setenv(AppContainerPidsLimitEnv, "0")
	cfg, err = Load(t.TempDir())
	if err != nil {
		t.Fatalf("0 must be accepted (disables the limit): %v", err)
	}
	if cfg.AppContainerMemoryLimitBytes != 0 || cfg.AppContainerCPULimitMillis != 0 || cfg.AppContainerPidsLimit != 0 {
		t.Fatalf("0 was not preserved: memory=%d cpu=%d pids=%d",
			cfg.AppContainerMemoryLimitBytes, cfg.AppContainerCPULimitMillis, cfg.AppContainerPidsLimit)
	}

	t.Setenv(AppContainerMemoryLimitBytesEnv, "-1")
	t.Setenv(AppContainerCPULimitMillisEnv, "0")
	t.Setenv(AppContainerPidsLimitEnv, "0")
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("negative app container memory limit was accepted")
	}
	t.Setenv(AppContainerMemoryLimitBytesEnv, "0")
	t.Setenv(AppContainerCPULimitMillisEnv, "-1")
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("negative app container cpu limit was accepted")
	}
	t.Setenv(AppContainerCPULimitMillisEnv, "0")
	t.Setenv(AppContainerPidsLimitEnv, "-1")
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("negative app container pids limit was accepted")
	}
}
