package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/hostforge/hostforge/internal/repository"
)

func (s *server) handlePlatformEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_limit"})
			return
		}
		limit = value
	}
	var cursor int64
	if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_cursor"})
			return
		}
		cursor = value
	}
	items, next, err := s.store.ListPlatformEventsFiltered(r.Context(), repository.PlatformEventFilter{
		ApplicationID: r.URL.Query().Get("application_id"), ServiceID: r.URL.Query().Get("service_id"), EventType: r.URL.Query().Get("type"),
		DateFrom: r.URL.Query().Get("date_from"), DateTo: r.URL.Query().Get("date_to"), Cursor: cursor, Limit: limit,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_events_failed"})
		return
	}
	nextCursor := ""
	if next > 0 {
		nextCursor = strconv.FormatInt(next, 10)
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": items, "next_cursor": nextCursor})
}
