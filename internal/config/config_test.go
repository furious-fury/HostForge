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
