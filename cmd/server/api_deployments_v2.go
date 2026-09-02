package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/furious-fury/HostForge/internal/services"
)

func deploymentToV2(item models.Deployment) map[string]any {
	return map[string]any{
		"id": item.ID, "service_id": item.ServiceID, "environment_id": item.EnvironmentID,
		"status": item.Status, "commit_hash": item.CommitHash, "logs_path": item.LogsPath,
		"image_ref": item.ImageRef, "error_message": item.ErrorMessage, "builder_kind": item.BuilderKind,
		"stack_kind": item.StackKind, "stack_label": item.StackLabel, "trigger": item.TriggerKind,
		"actor": item.Actor, "cancelled_at": item.CancelledAt, "rollback_of": item.RollbackOf,
		"branch":     item.Branch,
		"created_at": item.CreatedAt, "updated_at": item.UpdatedAt,
	}
}

func (s *server) deploymentToV2WithContext(ctx context.Context, item models.Deployment) map[string]any {
	row := deploymentToV2(item)
	service, err := s.store.GetService(ctx, item.ServiceID)
	if err != nil {
		return row
	}
	row["application_id"] = service.ApplicationID
	row["service_name"] = service.Name
	if application, appErr := s.store.GetApplication(ctx, service.ApplicationID); appErr == nil {
		row["application_name"] = application.Name
	}
	if environment, environmentErr := s.store.GetEnvironment(ctx, item.EnvironmentID); environmentErr == nil {
		row["environment_name"] = environment.Name
		row["environment_kind"] = environment.Kind
	}
	if binding, bindingErr := s.store.GetServiceEnvironment(ctx, item.ServiceID, item.EnvironmentID); bindingErr == nil {
		active := binding.ActiveDeploymentID == item.ID
		row["is_active"] = active
		if active {
			if domains, domainErr := s.store.ListServiceDomains(ctx, service.ApplicationID, item.EnvironmentID, item.ServiceID); domainErr == nil {
				urls := make([]string, 0, len(domains))
				for _, domain := range domains {
					urls = append(urls, "https://"+domain.DomainName)
				}
				row["urls"] = urls
				if len(urls) > 0 {
					row["public_url"] = urls[0]
				}
			}
		}
	}
	return row
}

func (s *server) handleDeploymentsV2Collection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_limit"})
			return
		}
		limit = parsed
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursor != "" {
		if _, err := s.store.GetServiceDeployment(r.Context(), cursor); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_cursor"})
			return
		}
	}
	status := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
	if status != "" && status != models.DeploymentQueued && status != models.DeploymentBuilding && status != models.DeploymentSuccess && status != models.DeploymentFailed && status != models.DeploymentCancelled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_status"})
		return
	}
	trigger := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("trigger")))
	if trigger != "" && trigger != "manual" && trigger != "webhook" && trigger != "redeploy" && trigger != "rollback" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_trigger"})
		return
	}
	dateFrom, fromTime, ok := parseDeploymentDateFilter(r.URL.Query().Get("date_from"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_date_from"})
		return
	}
	dateTo, toTime, ok := parseDeploymentDateFilter(r.URL.Query().Get("date_to"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_date_to"})
		return
	}
	if !fromTime.IsZero() && !toTime.IsZero() && fromTime.After(toTime) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_date_range"})
		return
	}
	items, nextCursor, err := s.store.ListServiceDeploymentsFiltered(r.Context(), repository.ServiceDeploymentFilter{
		ApplicationID: r.URL.Query().Get("application_id"),
		ServiceID:     r.URL.Query().Get("service_id"),
		EnvironmentID: r.URL.Query().Get("environment_id"),
		Status:        status,
		Trigger:       trigger,
		Branch:        r.URL.Query().Get("branch"),
		DateFrom:      dateFrom,
		DateTo:        dateTo,
		Cursor:        cursor,
		Limit:         limit,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_deployments_failed"})
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, s.deploymentToV2WithContext(r.Context(), item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": out, "next_cursor": nextCursor})
}

func parseDeploymentDateFilter(raw string) (string, time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", time.Time{}, true
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", time.Time{}, false
	}
	return parsed.UTC().Format(time.RFC3339), parsed, true
}

func (s *server) handleServiceDeployActionV2(w http.ResponseWriter, r *http.Request, serviceID, environmentID string) {
	target, err := services.ResolveDeployTarget(r.Context(), s.store, serviceID, environmentID)
	if err != nil {
		code := services.FirstPublicCode(err)
		if code == "" || code == "internal_error" {
			code = "service_environment_not_deployable"
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": code})
		return
	}
	// PrepareServiceDeploy enqueues the operation as its own last write; the
	// deploy runtime claims and runs it. Nothing here launches it directly
	// any more.
	job, err := services.PrepareServiceDeploy(r.Context(), s.cfg, s.store, target, "manual", "operator", "", "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": publicAPIError(err, "failed_to_accept_deployment")})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "deployment": deploymentToV2(job.Deployment)})
}

func (s *server) handleDeploymentV2Detail(w http.ResponseWriter, r *http.Request, deploymentID string) bool {
	item, err := s.store.GetServiceDeployment(r.Context(), deploymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "deployment_not_found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "deployment_lookup_failed"})
		}
		return true
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"deployment": s.deploymentToV2WithContext(r.Context(), item)})
		return true
	}
	return false
}

func (s *server) handleDeploymentCancelV2(w http.ResponseWriter, r *http.Request, deploymentID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	cancelled, err := s.store.CancelDeployment(r.Context(), deploymentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "cancel_deployment_failed"})
		return
	}
	if !cancelled {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "deployment_not_cancellable"})
		return
	}
	// CancelDeployment above is the gate: it produced the 409 case just
	// handled, it is what the UI reads, and it is what the log stream's
	// terminal poll observes. This is best-effort on top of it -- a running
	// deploy's operation row needs cancel_requested_at set so the runtime
	// actually stops it; a queued deploy's operation is cancelled outright
	// by the same call, though CancelDeployment above already made the
	// deployment itself terminal either way.
	if _, err := s.store.RequestOperationCancellation(r.Context(), deploymentID); err != nil {
		s.log.Warn("request operation cancellation failed", "deployment_id", deploymentID, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "deployment_id": strings.TrimSpace(deploymentID)})
}

func (s *server) handleDeploymentRedeployV2(w http.ResponseWriter, r *http.Request, deploymentID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	source, err := s.store.GetServiceDeployment(r.Context(), deploymentID)
	if err != nil || source.ServiceID == "" || source.EnvironmentID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "deployment_not_found"})
		return
	}
	if strings.TrimSpace(source.CommitHash) == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "deployment_commit_unavailable"})
		return
	}
	target, err := services.ResolveDeployTarget(r.Context(), s.store, source.ServiceID, source.EnvironmentID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "service_environment_not_deployable"})
		return
	}
	job, err := services.PrepareServiceDeploy(r.Context(), s.cfg, s.store, target, "redeploy", "operator", source.CommitHash, "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": publicAPIError(err, "failed_to_accept_deployment")})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "deployment": deploymentToV2(job.Deployment)})
}

func (s *server) handleDeploymentRollbackV2(w http.ResponseWriter, r *http.Request, deploymentID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	source, err := s.store.GetServiceDeployment(r.Context(), deploymentID)
	if err != nil || source.ServiceID == "" || source.EnvironmentID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "deployment_not_found"})
		return
	}
	if source.Status != models.DeploymentSuccess {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "rollback_source_not_successful"})
		return
	}
	if strings.TrimSpace(source.CommitHash) == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "deployment_commit_unavailable"})
		return
	}
	target, err := services.ResolveDeployTarget(r.Context(), s.store, source.ServiceID, source.EnvironmentID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "service_environment_not_deployable"})
		return
	}
	job, err := services.PrepareServiceDeploy(r.Context(), s.cfg, s.store, target, "rollback", "operator", source.CommitHash, source.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": publicAPIError(err, "failed_to_accept_rollback")})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "deployment": deploymentToV2(job.Deployment)})
}
