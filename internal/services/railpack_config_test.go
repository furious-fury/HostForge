package services

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hostforge/hostforge/internal/builder"
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

func TestValidateRailpackConfig_DisabledIsRejected(t *testing.T) {
	t.Parallel()
	if err := ValidateRailpackConfig(&config.Config{}); err == nil || !strings.Contains(err.Error(), "railpack is required") {
		t.Fatalf("got %v", err)
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

func TestNewRailpackAdapter_UsesEnabledConfiguration(t *testing.T) {
	t.Parallel()
	adapter, err := newRailpackAdapter(validRailpackConfig())
	if err != nil {
		t.Fatal(err)
	}
	if adapter == nil {
		t.Fatal("expected adapter")
	}
}

func TestRailpackLogSink_FormatsStructuredEvents(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	railpackLogSink(&out)(builder.Event{Phase: "build", Message: "building image"})
	if got := out.String(); got != "hostforge: railpack build: building image\n" {
		t.Fatalf("got %q", got)
	}
}
