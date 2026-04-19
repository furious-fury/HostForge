package main

import (
	"net/http"

	"github.com/hostforge/hostforge/internal/sysstatus"
)

func (s *server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	out := sysstatus.Gather(r.Context(), s.cfg)
	writeJSON(w, http.StatusOK, out)
}
