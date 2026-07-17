package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/repository"
	platformservices "github.com/hostforge/hostforge/internal/services"
)

func (s *server) handleDatabaseBackups(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/database-backups/"), "/"), "/")
	if len(parts) == 1 && parts[0] != "" && r.Method == http.MethodDelete {
		s.handleDeleteDatabaseBackup(w, r, parts[0])
		return
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] != "restore" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	backup, err := s.store.GetDatabaseBackup(r.Context(), parts[0])
	if err != nil || backup.Status != "success" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_backup_not_restorable"})
		return
	}
	var req struct {
		Mode              string `json:"mode"`
		TargetInstanceID  string `json:"target_instance_id"`
		Confirmation      string `json:"confirmation"`
		ApplicationID     string `json:"application_id"`
		EnvironmentID     string `json:"environment_id"`
		Name              string `json:"name"`
		ResourcePreset    string `json:"resource_preset"`
		CustomCPUMillis   int    `json:"custom_cpu_millis"`
		CustomMemoryBytes int64  `json:"custom_memory_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.Mode == "" {
		req.Mode = "new_service"
	}
	if req.Mode == "replace_current" {
		s.handleReplaceCurrentDatabaseRestore(w, r, backup, req.TargetInstanceID, req.Confirmation)
		return
	}
	if req.Mode != "new_service" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "database_restore_mode_invalid"})
		return
	}
	applicationID, environmentID, name, preset := strings.TrimSpace(req.ApplicationID), strings.TrimSpace(req.EnvironmentID), strings.TrimSpace(req.Name), strings.TrimSpace(req.ResourcePreset)
	if backup.DatabaseInstanceID != "" {
		if source, lookupErr := s.store.GetDatabaseInstance(r.Context(), backup.DatabaseInstanceID); lookupErr == nil {
			if sourceService, serviceErr := s.store.GetService(r.Context(), source.ServiceID); serviceErr == nil {
				if applicationID == "" {
					applicationID = sourceService.ApplicationID
				}
				if environmentID == "" {
					environmentID = source.EnvironmentID
				}
				if name == "" {
					name = sourceService.Name + " restore " + time.Now().UTC().Format("20060102-1504")
				}
				if preset == "" {
					preset = source.ResourcePreset
				}
				if preset == "custom" {
					if req.CustomCPUMillis == 0 {
						req.CustomCPUMillis = source.CPULimitMillis
					}
					if req.CustomMemoryBytes == 0 {
						req.CustomMemoryBytes = source.MemoryLimitBytes
					}
				}
			}
		}
	}
	if applicationID == "" || environmentID == "" || name == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "database_restore_target_required"})
		return
	}
	if preset == "" {
		preset = "standard"
	}
	created, err := platformservices.PrepareManagedDatabase(r.Context(), s.store, s.envSealer, platformservices.CreateManagedDatabaseInput{
		ApplicationID: applicationID, Name: name, Engine: backup.Engine, Version: backup.EngineVersion,
		EnvironmentIDs: []string{environmentID}, ResourcePreset: preset, Actor: "operator",
		CustomCPUMillis: req.CustomCPUMillis, CustomMemoryBytes: req.CustomMemoryBytes,
	})
	if err != nil || len(created.Instances) != 1 {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_restore_target_creation_failed"})
		return
	}
	operation, err := s.store.QueueDatabaseRestore(r.Context(), backup.ID, created.Instances[0].ID, "", "new_service", "operator", s.cfg.DatabaseTransferMaxPerHour)
	if err != nil {
		if errors.Is(err, repository.ErrDatabaseTransferLimited) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"status": "error", "error": "database_transfer_rate_limited"})
			return
		}
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_restore_queue_failed"})
		return
	}
	s.recordDatabaseOperationEvent(r, created.Instances[0], "database_restore", "Database restore to new service queued", operation.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "database_service": created.Service, "target_instance": created.Instances[0], "operation": operation})
}

func (s *server) handleDeleteDatabaseBackup(w http.ResponseWriter, r *http.Request, backupID string) {
	backup, err := s.store.PrepareDatabaseBackupDeletion(r.Context(), backupID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_backup_not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_backup_delete_unavailable"})
		return
	}
	if backup.Status == "deleting" {
		if backup.DestinationID == "" || strings.TrimSpace(backup.ObjectKey) == "" {
			writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_backup_object_metadata_missing"})
			return
		}
		destination, err := s.store.GetBackupDestinationSealed(r.Context(), backup.DestinationID)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "backup_destination_not_found"})
			return
		}
		client, err := destinationClient(r, destination, s)
		if err != nil || client.Delete(r.Context(), backup.ObjectKey) != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "database_backup_object_delete_failed"})
			return
		}
	}
	if err := s.store.DeleteDatabaseBackupRecord(r.Context(), backup.ID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_backup_record_delete_failed"})
		return
	}
	if backup.DatabaseInstanceID != "" {
		if instance, lookupErr := s.store.GetDatabaseInstance(r.Context(), backup.DatabaseInstanceID); lookupErr == nil {
			if service, serviceErr := s.store.GetService(r.Context(), instance.ServiceID); serviceErr == nil {
				_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: service.ApplicationID, ServiceID: service.ID, EnvironmentID: instance.EnvironmentID, EventType: "database_backup", Status: "deleted", Actor: "operator", Message: "Database backup deleted", Detail: backup.ID})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *server) handleReplaceCurrentDatabaseRestore(w http.ResponseWriter, r *http.Request, backup repository.DatabaseBackup, targetInstanceID, confirmation string) {
	target, err := s.store.GetDatabaseInstance(r.Context(), strings.TrimSpace(targetInstanceID))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_restore_target_not_found"})
		return
	}
	service, err := s.store.GetService(r.Context(), target.ServiceID)
	if err != nil || strings.TrimSpace(confirmation) != service.Name {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "database_restore_confirmation_mismatch"})
		return
	}
	policy, err := s.store.GetDatabaseBackupPolicy(r.Context(), target.ID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_restore_safety_destination_required"})
		return
	}
	safetyBackup, safetyOperation, err := s.store.QueueDatabaseBackup(r.Context(), target.ID, policy.DestinationID, "safety", "operator", max(policy.RetentionDays, 7), s.cfg.DatabaseTransferMaxPerHour)
	if err != nil {
		if errors.Is(err, repository.ErrDatabaseTransferLimited) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"status": "error", "error": "database_transfer_rate_limited"})
			return
		}
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_restore_safety_backup_queue_failed"})
		return
	}
	restoreOperation, err := s.store.QueueDatabaseRestore(r.Context(), backup.ID, target.ID, safetyBackup.ID, "replace_current", "operator", s.cfg.DatabaseTransferMaxPerHour)
	if err != nil {
		if errors.Is(err, repository.ErrDatabaseTransferLimited) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"status": "error", "error": "database_transfer_rate_limited"})
			return
		}
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "database_restore_queue_failed"})
		return
	}
	s.recordDatabaseOperationEvent(r, target, "database_restore", "Database replace-current restore queued with safety backup", restoreOperation.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "safety_backup": safetyBackup, "safety_operation": safetyOperation, "operation": restoreOperation})
}

func (s *server) recordDatabaseOperationEvent(r *http.Request, instance repository.DatabaseInstance, eventType, message, detail string) {
	if service, err := s.store.GetService(r.Context(), instance.ServiceID); err == nil {
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: service.ApplicationID, ServiceID: service.ID, EnvironmentID: instance.EnvironmentID, EventType: eventType, Status: "queued", Actor: "operator", Message: message, Detail: detail})
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
