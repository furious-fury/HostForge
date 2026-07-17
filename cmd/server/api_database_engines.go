package main

import (
	"net/http"

	"github.com/hostforge/hostforge/internal/databases"
)

func (s *server) handleDatabaseEngines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"engines":          databases.Catalog(),
		"resource_presets": databases.ResourcePresets(),
		"networking": map[string]any{
			"scope":                   "hostforge_environment",
			"public_access_available": false,
		},
	})
}
