package services

import (
	"testing"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/repository"
)

func TestDeployJobHealthConfigUsesDetectedNextRootForLegacyAutomaticDefault(t *testing.T) {
	cfg := &config.Config{HealthPath: "/"}
	job := DeployJob{Target: &DeployTarget{Service: repository.Service{
		DeployRuntime:   "auto",
		HealthCheckPath: "/health",
	}}}

	if got := job.healthConfig(cfg, "node_next").HealthPath; got != "/" {
		t.Fatalf("Next.js automatic health path = %q, want /", got)
	}
	if got := job.healthConfig(cfg, "node").HealthPath; got != "/health" {
		t.Fatalf("non-Next.js explicit health path = %q, want /health", got)
	}
}
