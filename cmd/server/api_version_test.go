package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/furious-fury/HostForge/internal/config"
)

// GET /api/version is registered without requireManagementAuth (ADR-0002
// §24.4): it must answer with zero auth headers, and it must expose only
// build metadata — not the config/paths/auth blocks /api/settings carries.
func TestHandleVersionGetUnauthenticated(t *testing.T) {
	s := &server{log: slog.New(slog.NewTextHandler(io.Discard, nil)), cfg: &config.Config{}}
	r := httptest.NewRequest("GET", "/api/version", nil)
	w := httptest.NewRecorder()
	s.handleVersionGet(w, r)

	if w.Code != 200 {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	for _, key := range []string{
		"version", "version_display", "commit", "build_time",
		"go_version", "os", "arch", "pid", "started_at", "uptime_seconds",
	} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing key %q in response: %v", key, body)
		}
	}

	for _, leak := range []string{"data_dir", "auth", "network", "caddy", "db_path"} {
		if _, ok := body[leak]; ok {
			t.Errorf("unexpected key %q leaked into unauthenticated /api/version response", leak)
		}
	}
}

// buildVersionInfo backs both /api/version and /api/settings' "build"
// section (cmd/server/api_settings.go: buildSettingsPayload calls the same
// helper) — a single source of truth by construction, not re-verified here
// since exercising buildSettingsPayload needs a full repository.Store.
func TestBuildVersionInfoHasExpectedShape(t *testing.T) {
	info := buildVersionInfo()
	for _, key := range []string{
		"version", "version_display", "commit", "build_time",
		"go_version", "os", "arch", "pid", "started_at", "uptime_seconds",
	} {
		if _, ok := info[key]; !ok {
			t.Errorf("missing key %q in buildVersionInfo(): %v", key, info)
		}
	}
}
