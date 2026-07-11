package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/hostforge/hostforge/internal/hostmetrics"
)

type hostSnapshotCache struct {
	mu     sync.Mutex
	until  time.Time
	status int
	body   []byte
}

func (c *hostSnapshotCache) get(now time.Time) ([]byte, int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.body) > 0 && now.Before(c.until) {
		return c.body, c.status, true
	}
	return nil, 0, false
}

func (c *hostSnapshotCache) set(now time.Time, ttl time.Duration, status int, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.until = now.Add(ttl)
	c.status = status
	c.body = append([]byte(nil), body...)
}

func (s *server) handleHostSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	if s.hostSampler == nil {
		writeJSON(w, http.StatusOK, map[string]any{"supported": false, "error_code": "disabled"})
		return
	}
	if !s.hostSampler.Supported() {
		writeJSON(w, http.StatusOK, map[string]any{"supported": false, "error_code": "unsupported_os"})
		return
	}

	now := time.Now()
	if body, status, ok := s.hostSnapCache.get(now); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
		return
	}

	if !s.hostSampler.HasSamples() {
		payload := map[string]string{
			"error":  "warming_up",
			"detail": "host metrics have no samples yet; wait for the next collection tick",
		}
		writeJSON(w, http.StatusServiceUnavailable, payload)
		return
	}

	snap := s.hostSampler.Latest()
	payload := map[string]any{
		"supported": true,
		"sample":    snap,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode_failed"})
		return
	}
	s.hostSnapCache.set(now, time.Second, http.StatusOK, raw)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (s *server) handleHostHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	if s.hostSampler == nil {
		writeJSON(w, http.StatusOK, map[string]any{"supported": false, "error_code": "disabled", "samples": []hostmetrics.Sample{}})
		return
	}
	if !s.hostSampler.Supported() {
		writeJSON(w, http.StatusOK, map[string]any{"supported": false, "error_code": "unsupported_os", "samples": []hostmetrics.Sample{}})
		return
	}

	points := 0
	if v := r.URL.Query().Get("points"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			points = n
		}
	}
	if points > 720 {
		points = 720
	}
	hist := s.hostSampler.History(points)
	writeJSON(w, http.StatusOK, map[string]any{"supported": true, "samples": hist})
}
