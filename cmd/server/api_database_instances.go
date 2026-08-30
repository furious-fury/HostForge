package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/databases"
	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/repository"
	platformservices "github.com/furious-fury/HostForge/internal/services"
)

func (s *server) handleDatabaseInstances(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/database-instances/"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	action := strings.ToLower(parts[1])
	if action == "external-access" && r.Method == http.MethodGet {
		s.handleDatabaseInstanceExternalAccess(w, r, parts[0])
		return
	}
	if action == "external-connections" && r.Method == http.MethodPost {
		s.handleDatabaseInstanceExternalConnections(w, r, parts[0])
		return
	}
	if action == "upgrade" && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
		s.handleDatabaseInstanceUpgrade(w, r, parts[0])
		return
	}
	if action == "backup-policy" && (r.Method == http.MethodGet || r.Method == http.MethodPut) {
		s.handleDatabaseBackupPolicy(w, r, parts[0])
		return
	}
	if action == "backups" && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
		s.handleDatabaseInstanceBackups(w, r, parts[0])
		return
	}
	if action == "bindings" && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
		s.handleDatabaseInstanceBindings(w, r, parts[0])
		return
	}
	if r.Method == http.MethodGet && (action == "logs" || action == "metrics") {
		s.handleDatabaseInstanceDiagnostics(w, r, parts[0], action)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	operationType := action
	if action == "rotate-credentials" {
		operationType = "rotate_credentials"
	}
	if operationType != "start" && operationType != "stop" && operationType != "restart" && operationType != "rotate_credentials" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	operation, err := s.store.QueueDatabaseInstanceOperation(r.Context(), parts[0], operationType, "operator")
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_runtime_action_unavailable"})
		return
	}
	if service, lookupErr := s.store.GetService(r.Context(), operation.ServiceID); lookupErr == nil {
		message := "Database " + strings.ReplaceAll(operationType, "_", " ") + " queued"
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: service.ApplicationID, ServiceID: service.ID, EnvironmentID: instanceEnvironmentID(s.store, r, operation.DatabaseInstanceID), EventType: "database", Status: "queued", Actor: "operator", Message: message, Detail: operation.ID})
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "operation": operation})
}

func instanceEnvironmentID(store *repository.Store, r *http.Request, instanceID string) string {
	instance, err := store.GetDatabaseInstance(r.Context(), instanceID)
	if err != nil {
		return ""
	}
	return instance.EnvironmentID
}

const databaseUpgradeBackupMaxAge = 24 * time.Hour

func (s *server) handleDatabaseInstanceUpgrade(w http.ResponseWriter, r *http.Request, instanceID string) {
	instance, err := s.store.GetDatabaseInstance(r.Context(), instanceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_instance_not_found"})
		return
	}
	databaseService, err := s.store.GetDatabaseService(r.Context(), instance.ServiceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_service_not_found"})
		return
	}
	_, version, found := databases.FindVersion(databaseService.Engine, instance.EngineVersion)
	available, reason := false, "catalog_image_current"
	if !found || strings.TrimSpace(version.ImageRef) == "" {
		reason = "catalog_version_unavailable"
	} else if version.ImageRef != instance.ImageRef {
		available, reason = true, ""
	}
	backup, backupErr := s.store.LatestSuccessfulDatabaseBackup(r.Context(), instance.ID)
	backupAge := time.Since(backup.CompletedAt)
	backupRecent := backupErr == nil && !backup.CompletedAt.IsZero() && backupAge >= 0 && backupAge <= databaseUpgradeBackupMaxAge
	var latestBackup any
	if backupErr == nil {
		latestBackup = backup
	}
	ready := available && backupRecent && instance.Status == "healthy" && instance.DesiredState == "running" && instance.DockerContainerID != ""
	if r.Method == http.MethodGet {
		if available && !backupRecent {
			reason = "recent_backup_required"
		} else if available && !ready {
			reason = "database_not_healthy"
		}
		writeJSON(w, http.StatusOK, map[string]any{"available": available, "ready": ready, "reason": reason, "engine_version": instance.EngineVersion, "current_image_ref": instance.ImageRef, "target_image_ref": version.ImageRef, "backup_max_age_hours": int(databaseUpgradeBackupMaxAge.Hours()), "latest_backup": latestBackup})
		return
	}
	if !ready {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": reason})
		return
	}
	operation, err := s.store.QueueDatabaseUpgrade(r.Context(), instance.ID, instance.EngineVersion, version.ImageRef, "operator")
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_upgrade_unavailable"})
		return
	}
	if service, lookupErr := s.store.GetService(r.Context(), instance.ServiceID); lookupErr == nil {
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: service.ApplicationID, ServiceID: service.ID, EnvironmentID: instance.EnvironmentID, EventType: "database", Status: "queued", Actor: "operator", Message: "Database patch upgrade queued", Detail: operation.ID})
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "operation": operation})
}

func (s *server) handleDatabaseInstanceBindings(w http.ResponseWriter, r *http.Request, instanceID string) {
	if r.Method == http.MethodGet {
		items, err := s.store.ListDatabaseBindings(r.Context(), instanceID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_instance_not_found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"bindings": items})
		return
	}
	var req struct {
		ConsumerServiceID string `json:"consumer_service_id"`
		VariableKey       string `json:"variable_key"`
		ReplaceExisting   bool   `json:"replace_existing"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	item, err := s.store.CreateDatabaseBinding(r.Context(), instanceID, req.ConsumerServiceID, req.VariableKey, req.ReplaceExisting)
	if err != nil {
		code := "database_binding_invalid"
		if errors.Is(err, repository.ErrDatabaseBindingConflict) {
			code = "database_binding_variable_conflict"
		}
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": code})
		return
	}
	s.recordDatabaseBindingEvent(r, item, "created", "Database application binding created")
	writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "binding": item})
}

func (s *server) recordDatabaseBindingEvent(r *http.Request, binding repository.DatabaseBinding, status, message string) {
	instance, err := s.store.GetDatabaseInstance(r.Context(), binding.DatabaseInstanceID)
	if err != nil {
		return
	}
	service, err := s.store.GetService(r.Context(), instance.ServiceID)
	if err != nil {
		return
	}
	_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: service.ApplicationID, ServiceID: service.ID, EnvironmentID: instance.EnvironmentID, EventType: "database_binding", Status: status, Actor: "operator", Message: message, Detail: binding.ConsumerServiceID + ":" + binding.VariableKey})
}

func (s *server) handleDatabaseBindings(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/database-bindings/"), "/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			ConsumerServiceID string `json:"consumer_service_id"`
			VariableKey       string `json:"variable_key"`
			ReplaceExisting   bool   `json:"replace_existing"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
			return
		}
		item, err := s.store.UpdateDatabaseBinding(r.Context(), id, req.ConsumerServiceID, req.VariableKey, req.ReplaceExisting)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_binding_not_found"})
			return
		}
		if err != nil {
			code := "database_binding_invalid"
			if errors.Is(err, repository.ErrDatabaseBindingConflict) {
				code = "database_binding_variable_conflict"
			}
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": code})
			return
		}
		s.recordDatabaseBindingEvent(r, item, "updated", "Database application binding updated")
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "binding": item})
	case http.MethodDelete:
		item, lookupErr := s.store.GetDatabaseBinding(r.Context(), id)
		if lookupErr != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_binding_not_found"})
			return
		}
		if err := s.store.DeleteDatabaseBinding(r.Context(), id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_binding_not_found"})
			return
		}
		s.recordDatabaseBindingEvent(r, item, "deleted", "Database application binding deleted")
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
	}
}

func (s *server) handleDatabaseBackupPolicy(w http.ResponseWriter, r *http.Request, instanceID string) {
	if _, err := s.store.GetDatabaseInstance(r.Context(), instanceID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_instance_not_found"})
		return
	}
	if r.Method == http.MethodGet {
		policy, err := s.store.GetDatabaseBackupPolicy(r.Context(), instanceID)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{"policy": nil})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "backup_policy_lookup_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"policy": policy})
		return
	}
	var req struct {
		DestinationID string `json:"destination_id"`
		Enabled       bool   `json:"enabled"`
		Schedule      string `json:"schedule"`
		Timezone      string `json:"timezone"`
		RetentionDays int    `json:"retention_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	if strings.TrimSpace(req.Schedule) == "" {
		req.Schedule = "0 2 * * *"
	}
	if strings.TrimSpace(req.Timezone) == "" {
		req.Timezone = "UTC"
	}
	if req.RetentionDays == 0 {
		req.RetentionDays = 30
	}
	next, err := platformservices.NextDatabaseBackupSchedule(req.Schedule, req.Timezone, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "backup_schedule_invalid"})
		return
	}
	policy, err := s.store.UpsertDatabaseBackupPolicy(r.Context(), instanceID, req.DestinationID, req.Enabled, req.Schedule, req.Timezone, req.RetentionDays, next)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "backup_policy_invalid"})
		return
	}
	if instance, lookupErr := s.store.GetDatabaseInstance(r.Context(), instanceID); lookupErr == nil {
		if service, serviceErr := s.store.GetService(r.Context(), instance.ServiceID); serviceErr == nil {
			_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: service.ApplicationID, ServiceID: service.ID, EnvironmentID: instance.EnvironmentID, EventType: "database_backup_policy", Status: "updated", Actor: "operator", Message: "Database backup policy updated", Detail: policy.DestinationID})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved", "policy": policy})
}

func (s *server) handleDatabaseInstanceBackups(w http.ResponseWriter, r *http.Request, instanceID string) {
	if r.Method == http.MethodGet {
		items, err := s.store.ListDatabaseBackups(r.Context(), instanceID, 50)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_database_backups_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"backups": items})
		return
	}
	policy, err := s.store.GetDatabaseBackupPolicy(r.Context(), instanceID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "backup_destination_required"})
		return
	}
	backup, operation, err := s.store.QueueDatabaseBackup(r.Context(), instanceID, policy.DestinationID, "manual", "operator", policy.RetentionDays, s.cfg.DatabaseTransferMaxPerHour)
	if err != nil {
		if errors.Is(err, repository.ErrDatabaseTransferLimited) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"status": "error", "error": "database_transfer_rate_limited"})
			return
		}
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_backup_unavailable"})
		return
	}
	if instance, lookupErr := s.store.GetDatabaseInstance(r.Context(), instanceID); lookupErr == nil {
		s.recordDatabaseOperationEvent(r, instance, "database_backup", "Manual database backup queued", operation.ID)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "backup": backup, "operation": operation})
}

func (s *server) handleDatabaseInstanceDiagnostics(w http.ResponseWriter, r *http.Request, instanceID, kind string) {
	instance, err := s.store.GetDatabaseInstance(r.Context(), instanceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_instance_not_found"})
		return
	}
	if strings.TrimSpace(instance.DockerContainerID) == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_container_not_provisioned"})
		return
	}
	client, err := docker.NewClient(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "docker_unavailable"})
		return
	}
	defer client.Close()
	inspection, err := docker.InspectManagedContainer(r.Context(), client, instance.DockerContainerID)
	if err != nil || inspection.Labels[docker.ResourceTypeLabel] != "database-container" || inspection.Labels[docker.InstanceIDLabel] != instance.ID {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_container_ownership_mismatch"})
		return
	}
	if kind == "logs" {
		tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
		logs, err := docker.ReadContainerLogs(r.Context(), client, instance.DockerContainerID, tail)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "database_logs_unavailable"})
			return
		}
		credential, err := s.store.GetDatabaseCredentialSealed(r.Context(), instance.ID)
		if err != nil || s.envSealer == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "database_logs_redaction_unavailable"})
			return
		}
		password, passwordErr := s.envSealer.Open(credential.PasswordCT)
		adminPassword, adminErr := s.envSealer.Open(credential.AdminPasswordCT)
		pendingPassword, pendingErr := s.envSealer.Open(credential.PendingPasswordCT)
		if passwordErr != nil || adminErr != nil || pendingErr != nil && len(credential.PendingPasswordCT) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "database_logs_redaction_unavailable"})
			return
		}
		decrypted := [][]byte{password, adminPassword, pendingPassword}
		defer func() {
			for _, secret := range decrypted {
				for index := range secret {
					secret[index] = 0
				}
			}
		}()
		databaseService, err := s.store.GetDatabaseService(r.Context(), instance.ServiceID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "database_logs_redaction_unavailable"})
			return
		}
		logs = platformservices.RedactDatabaseLogs(databaseService.Engine, logs, decrypted...)
		writeJSON(w, http.StatusOK, map[string]any{"instance_id": instance.ID, "logs": logs})
		return
	}
	metric, err := docker.CollectContainerMetric(r.Context(), client, instance.DockerContainerID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "database_metrics_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"instance_id": instance.ID, "metric": map[string]any{
		"cpu_percent": metric.CPUPercent, "memory_bytes": metric.MemoryBytes,
		"network_rx_bytes": metric.NetworkRX, "network_tx_bytes": metric.NetworkTX,
		"sampled_at": metric.SampledAt,
	}})
}
