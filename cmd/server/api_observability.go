package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/furious-fury/HostForge/internal/sysstatus"
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
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
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
	limit, cursor, ok := parseObservabilityPage(w, r, 100)
	if !ok {
		return
	}
	statusClass := strings.TrimSpace(r.URL.Query().Get("status_class"))
	if statusClass != "" && statusClass != "success" && statusClass != "client_error" && statusClass != "server_error" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_status_class"})
		return
	}
	rows, next, err := s.store.ListHTTPRequestsFiltered(r.Context(), repository.HTTPRequestFilter{
		ApplicationID: r.URL.Query().Get("application_id"), ServiceID: r.URL.Query().Get("service_id"),
		EnvironmentID: r.URL.Query().Get("environment_id"), Method: r.URL.Query().Get("method"), StatusClass: statusClass,
		DateFrom: r.URL.Query().Get("date_from"), DateTo: r.URL.Query().Get("date_to"), Cursor: cursor, Limit: limit,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "observability_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": rows, "next_cursor": formatObservabilityCursor(next)})
}

func (s *server) handleObservabilityDeploySteps(w http.ResponseWriter, r *http.Request) {
	limit, cursor, ok := parseObservabilityPage(w, r, 200)
	if !ok {
		return
	}
	rows, next, err := s.store.ListDeployStepsFiltered(r.Context(), repository.DeployStepFilter{
		ApplicationID: r.URL.Query().Get("application_id"), ServiceID: r.URL.Query().Get("service_id"),
		EnvironmentID: r.URL.Query().Get("environment_id"), Status: r.URL.Query().Get("status"),
		DateFrom: r.URL.Query().Get("date_from"), DateTo: r.URL.Query().Get("date_to"), Cursor: cursor, Limit: limit,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "observability_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deploy_steps": rows, "next_cursor": formatObservabilityCursor(next)})
}

func parseObservabilityPage(w http.ResponseWriter, r *http.Request, defaultLimit int) (int, int64, bool) {
	limit := defaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_limit"})
			return 0, 0, false
		}
		limit = value
	}
	var cursor int64
	if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_cursor"})
			return 0, 0, false
		}
		cursor = value
	}
	return limit, cursor, true
}

func formatObservabilityCursor(cursor int64) string {
	if cursor < 1 {
		return ""
	}
	return strconv.FormatInt(cursor, 10)
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
