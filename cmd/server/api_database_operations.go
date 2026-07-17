package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

func (s *server) handleDatabaseOperations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	operationID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/database-operations/"), "/")
	if operationID == "" || strings.Contains(operationID, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	operation, err := s.store.GetDatabaseOperation(r.Context(), operationID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_operation_not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "database_operation_lookup_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"operation": operation})
}
