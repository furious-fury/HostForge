package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/hostforge/hostforge/internal/sysstatus"
)

func (s *server) handleObservabilityRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	trim := strings.TrimPrefix(r.URL.Path, "/api/observability/")
	trim = strings.Trim(trim, "/")
	switch trim {
	case "summary":
		s.handleObservabilitySummary(w, r)
	case "requests":
		s.handleObservabilityRequests(w, r)
	case "deploy-steps":
		s.handleObservabilityDeploySteps(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *server) handleObservabilitySummary(w http.ResponseWriter, r *http.Request) {
	sum, err := s.store.SummarizeObservability(r.Context(), 24)
	if err != nil {
		s.requestLog(r).Warn("observability summary failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "observability_summary_failed"})
		return
	}
	sys := sysstatus.GatherCached(r.Context(), s.cfg)
	writeJSON(w, http.StatusOK, map[string]any{
		"summary": sum,
		"system":  sys,
	})
}

func (s *server) handleObservabilityRequests(w http.ResponseWriter, r *http.Request) {
	limit := parseQueryInt(r, "limit", 100)
	rows, err := s.store.ListRecentHTTPRequests(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "observability_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": rows})
}

func (s *server) handleObservabilityDeploySteps(w http.ResponseWriter, r *http.Request) {
	limit := parseQueryInt(r, "limit", 200)
	rows, err := s.store.ListRecentDeploySteps(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "observability_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deploy_steps": rows})
}

func (s *server) handleDeploymentSteps(w http.ResponseWriter, r *http.Request, deploymentID string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if _, err := s.store.GetDeploymentByID(r.Context(), deploymentID); err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "deployment_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "deployment_lookup_failed"})
		return
	}
	rows, err := s.store.ListDeployStepsByDeployment(r.Context(), deploymentID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "observability_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployment_id": deploymentID, "steps": rows})
}
