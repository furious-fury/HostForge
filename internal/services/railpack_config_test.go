package services

import (
	"strings"
	"testing"

	"github.com/hostforge/hostforge/internal/config"
)

func validRailpackConfig() *config.Config {
	return &config.Config{
		RailpackEnabled:          true,
		RailpackBin:              "railpack",
		RailpackVersion:          "v0.23.0",
		RailpackFrontendImage:    "ghcr.io/railwayapp/railpack-frontend@sha256:abcdef",
		BuildKitBin:              "buildctl",
		BuildKitAddress:          "unix:///run/buildkit/buildkitd.sock",
		RailpackArtifactsDir:     "/var/lib/hostforge/railpack",
		RailpackBuildConcurrency: 1,
		RailpackMinFreeDiskBytes: 10 * 1024 * 1024 * 1024,
	}
}

func TestValidateRailpackConfig_DisabledAllowsExistingBaseline(t *testing.T) {
	t.Parallel()
	if err := ValidateRailpackConfig(&config.Config{}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRailpackConfig_RequiresPinnedFrontend(t *testing.T) {
	t.Parallel()
	cfg := validRailpackConfig()
	cfg.RailpackFrontendImage = "ghcr.io/railwayapp/railpack-frontend:latest"
	err := ValidateRailpackConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "digest-pinned") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRailpackConfig_RequiresCompleteEnabledConfiguration(t *testing.T) {
	t.Parallel()
	cfg := validRailpackConfig()
	cfg.RailpackVersion = ""
	err := ValidateRailpackConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "helper binary and version") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRailpackConfig_AcceptsCompleteConfiguration(t *testing.T) {
	t.Parallel()
	if err := ValidateRailpackConfig(validRailpackConfig()); err != nil {
		t.Fatal(err)
	}
}
