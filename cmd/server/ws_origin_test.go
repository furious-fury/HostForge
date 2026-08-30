package main

import (
	"net/http/httptest"
	"testing"

	"github.com/furious-fury/HostForge/internal/config"
)

// checkWSOrigin gates the deployment log WebSocket upgrade (ADR-0002 §8.3).
// The old CheckOrigin unconditionally returned true; these cases pin the
// replacement so a future edit can't silently widen it back open.
func TestCheckWSOrigin(t *testing.T) {
	s := &server{cfg: &config.Config{PlatformDomainBase: "apps.example.com"}}

	cases := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{"same origin", "hostforge.internal:8080", "https://hostforge.internal:8080", true},
		{"platform domain", "hostforge.internal", "https://apps.example.com", true},
		{"subdomain of platform domain", "hostforge.internal", "https://web-myapp.apps.example.com", true},
		{"loopback", "hostforge.internal", "http://127.0.0.1:5173", true},
		{"localhost", "hostforge.internal", "http://localhost:5173", true},
		{"unrelated origin rejected", "hostforge.internal", "https://evil.example.net", false},
		{"lookalike suffix rejected", "hostforge.internal", "https://notapps.example.com", false},
		{"empty origin allowed (non-browser client)", "hostforge.internal", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/deployments/d1/logs/live", nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if got := s.checkWSOrigin(r); got != tc.want {
				t.Errorf("checkWSOrigin(origin=%q, host=%q) = %v, want %v", tc.origin, tc.host, got, tc.want)
			}
		})
	}
}
