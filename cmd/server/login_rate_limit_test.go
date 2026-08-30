package main

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
)

// /auth/session had no rate limiting (ADR-0002 §8.4); an attacker with
// network access could brute-force HOSTFORGE_API_TOKEN with unlimited
// attempts. This pins the fix: the limiter gates every POST, including
// ones with a correct token, before the (N+1)th attempt in a window.
func TestHandleSessionCreateRateLimited(t *testing.T) {
	s := &server{
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg: &config.Config{
			APIToken:          "correct-token",
			SessionSecret:     "0123456789abcdef",
			SessionCookieName: "hf_session",
			SessionTTLMinutes: 60,
		},
		loginLimiter: newFixedWindowLimiter(2, time.Minute),
	}

	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "/auth/session", nil)
		r.Header.Set("Authorization", "Bearer correct-token")
		r.RemoteAddr = "203.0.113.9:5555"
		w := httptest.NewRecorder()
		s.handleSessionCreate(w, r)
		if w.Code != 200 {
			t.Fatalf("attempt %d: status=%d body=%s", i+1, w.Code, w.Body.String())
		}
	}

	r := httptest.NewRequest("POST", "/auth/session", nil)
	r.Header.Set("Authorization", "Bearer correct-token")
	r.RemoteAddr = "203.0.113.9:5555"
	w := httptest.NewRecorder()
	s.handleSessionCreate(w, r)
	if w.Code != 429 {
		t.Fatalf("3rd attempt: status=%d, want 429; body=%s", w.Code, w.Body.String())
	}

	// A different IP is unaffected by the first IP's window.
	r2 := httptest.NewRequest("POST", "/auth/session", nil)
	r2.Header.Set("Authorization", "Bearer correct-token")
	r2.RemoteAddr = "198.51.100.4:5555"
	w2 := httptest.NewRecorder()
	s.handleSessionCreate(w2, r2)
	if w2.Code != 200 {
		t.Fatalf("different IP: status=%d, want 200; body=%s", w2.Code, w2.Body.String())
	}
}
