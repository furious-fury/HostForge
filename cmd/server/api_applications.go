package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/databases"
	"github.com/furious-fury/HostForge/internal/repository"
	platformservices "github.com/furious-fury/HostForge/internal/services"
)

func (s *server) handleApplications(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/applications" {
		switch r.Method {
		case http.MethodGet:
			items, err := s.store.ListApplicationSummaries(r.Context())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_applications_failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"applications": items})
		case http.MethodPost:
			var req struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
				return
			}
			item, err := s.store.CreateApplication(r.Context(), req.Name, req.Description)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "create_application_failed"})
				return
			}
			_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: item.ID, EventType: "application", Status: "created", Actor: "operator", Message: "Application created", Detail: item.Name})
			writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "application": item})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/applications/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	app, err := s.store.GetApplication(r.Context(), parts[0])
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "application_not_found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "application_lookup_failed"})
		}
		return
	}
	if (len(parts) == 4 || len(parts) == 5) && parts[1] == "environments" {
		resourceID := ""
		if len(parts) == 5 {
			resourceID = parts[4]
		}
		switch parts[3] {
		case "domains":
			s.handleServiceDomains(w, r, app.ID, parts[2], resourceID)
			return
		case "variables":
			s.handleEnvironmentVariables(w, r, app.ID, parts[2], resourceID)
			return
		}
	}
	if len(parts) == 2 && parts[1] == "services" && r.Method == http.MethodPost {
		in, _, ok := decodeServiceRequest(w, r, app.ID, nil)
		if !ok {
			return
		}
		if !s.validateServiceSource(w, r, in) {
			return
		}
		if in.InitialEnvironmentID != "" {
			environment, err := s.store.GetEnvironment(r.Context(), in.InitialEnvironmentID)
			if err != nil || environment.ApplicationID != app.ID {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"status": "error", "error": "environment_not_found", "fields": map[string]string{"environment_id": "not_found"}})
				return
			}
			if strings.TrimSpace(in.InitialBranch) == "" {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"status": "error", "error": "service_environment_branch_required", "fields": map[string]string{"branch": "required"}})
				return
			}
			candidate := repository.Service{ApplicationID: app.ID, RepoURL: in.RepoURL, GitHubInstallationID: in.GitHubInstallationID}
			if !s.validateServiceBranch(w, r, candidate, in.InitialBranch) {
				return
			}
		}
		item, err := s.store.CreateService(r.Context(), in)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: app.ID, ServiceID: item.ID, EventType: "service", Status: "created", Actor: "operator", Message: "Service created", Detail: item.Name})
		response := map[string]any{"status": "created", "service": item}
		if in.InitialEnvironmentID != "" {
			if binding, err := s.store.GetServiceEnvironment(r.Context(), item.ID, in.InitialEnvironmentID); err == nil {
				response["binding"] = binding
			}
		}
		writeJSON(w, http.StatusCreated, response)
		return
	}
	if len(parts) == 2 && parts[1] == "database-services" && r.Method == http.MethodPost {
		var req struct {
			Name                string   `json:"name"`
			Engine              string   `json:"engine"`
			Version             string   `json:"version"`
			EnvironmentIDs      []string `json:"environment_ids"`
			ResourcePreset      string   `json:"resource_preset"`
			CustomCPUMillis     int      `json:"custom_cpu_millis"`
			CustomMemoryBytes   int64    `json:"custom_memory_bytes"`
			BackupEnabled       bool     `json:"backup_enabled"`
			BackupDestinationID string   `json:"backup_destination_id"`
			Connections         []struct {
				ServiceID       string `json:"service_id"`
				VariableKey     string `json:"variable_key"`
				ReplaceExisting bool   `json:"replace_existing"`
			} `json:"connections"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
			return
		}
		if req.BackupEnabled {
			if _, err := s.store.GetBackupDestination(r.Context(), req.BackupDestinationID); err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "backup_destination_required"})
				return
			}
		}
		preset, presetFound := databases.FindResourcePreset(strings.ToLower(strings.TrimSpace(req.ResourcePreset)))
		if presetFound {
			cpuMillis, memoryBytes := preset.CPULimitMillis, preset.MemoryLimitBytes
			if preset.ID == "custom" {
				cpuMillis, memoryBytes = req.CustomCPUMillis, req.CustomMemoryBytes
			}
			capacity := s.managedDatabaseCapacity(r.Context())
			instanceCount := len(req.EnvironmentIDs)
			if capacity.Available && instanceCount > 0 &&
				(cpuMillis*instanceCount > capacity.CPUAvailableMillis || memoryBytes*int64(instanceCount) > capacity.MemoryAvailableBytes) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"status": "error", "error": "database_resource_capacity_exceeded", "resource_capacity": capacity,
				})
				return
			}
		}
		connections := make([]platformservices.DatabaseConnectionInput, 0, len(req.Connections))
		for _, connection := range req.Connections {
			connections = append(connections, platformservices.DatabaseConnectionInput{
				ConsumerServiceID: connection.ServiceID, VariableKey: connection.VariableKey,
				ReplaceExisting: connection.ReplaceExisting,
			})
		}
		automaticPublicAccess := strings.EqualFold(strings.TrimSpace(req.Engine), "postgresql") && s.cfg.DatabaseGatewaysEnabled
		clientCIDR := ""
		if automaticPublicAccess {
			// Keep endpoint creation and the initial grants on the same side of a
			// platform-domain transition. A later data-plane failure still retains
			// the healthy private database for an explicit retry.
			s.databaseGatewayDomainMu.RLock()
			defer s.databaseGatewayDomainMu.RUnlock()
			clientCIDR = requestCIDR(r)
			if clientCIDR == "" {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "initial_public_access_source_ip_unavailable"})
				return
			}
			if _, err := s.ensurePostgreSQLGatewayEndpoint(r); err != nil {
				writeDatabaseGatewayError(w, err)
				return
			}
		}
		created, err := platformservices.PrepareManagedDatabase(r.Context(), s.store, s.envSealer, platformservices.CreateManagedDatabaseInput{
			ApplicationID: app.ID, Name: req.Name, Engine: req.Engine, Version: req.Version,
			EnvironmentIDs: req.EnvironmentIDs, ResourcePreset: req.ResourcePreset,
			CustomCPUMillis: req.CustomCPUMillis, CustomMemoryBytes: req.CustomMemoryBytes,
			Connections: connections, Actor: "operator",
		})
		if err != nil {
			code := publicAPIError(err, "create_database_service_failed")
			status := http.StatusUnprocessableEntity
			if code == "env_encryption_key_missing" {
				status = http.StatusServiceUnavailable
			}
			payload := map[string]any{"status": "error", "error": code}
			if code == "database_service_name_conflict" {
				status = http.StatusConflict
				payload["message"] = "A service with this name already exists in the application."
				payload["fields"] = map[string]string{"name": "already_in_use"}
			}
			writeJSON(w, status, payload)
			return
		}
		if req.BackupEnabled {
			next, _ := platformservices.NextDatabaseBackupSchedule("0 2 * * *", "UTC", time.Now().UTC())
			for _, instance := range created.Instances {
				if _, err := s.store.UpsertDatabaseBackupPolicy(r.Context(), instance.ID, req.BackupDestinationID, true, "0 2 * * *", "UTC", 30, next); err != nil {
					_ = s.store.DeleteService(r.Context(), created.Service.ID)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "database_backup_policy_creation_failed"})
					return
				}
			}
		}
		initialPublicAccess := []map[string]any{}
		initialPublicAccessErrors := []map[string]string{}
		if automaticPublicAccess {
			for _, instance := range created.Instances {
				environment, environmentErr := s.store.GetEnvironment(r.Context(), instance.EnvironmentID)
				if environmentErr != nil {
					initialPublicAccessErrors = append(initialPublicAccessErrors, map[string]string{"environment_id": instance.EnvironmentID, "error": "initial_public_access_source_ip_unavailable"})
					continue
				}
				connection, operation, connectionErr := s.store.CreateDatabaseExternalConnection(r.Context(), instance.ID, repository.CreateExternalConnectionInput{Name: environment.Name + " public access", PermissionProfile: "read_write", CIDRs: []string{clientCIDR}, Actor: "operator:database-create"})
				if connectionErr != nil {
					initialPublicAccessErrors = append(initialPublicAccessErrors, map[string]string{"environment_id": instance.EnvironmentID, "environment_name": environment.Name, "error": publicAPIError(connectionErr, "initial_public_access_queue_failed")})
					continue
				}
				initialPublicAccess = append(initialPublicAccess, map[string]any{"environment_id": instance.EnvironmentID, "environment_name": environment.Name, "connection": connection, "operation": operation})
			}
		}
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{
			ApplicationID: app.ID, ServiceID: created.Service.ID, EventType: "database",
			Status: "queued", Actor: "operator", Message: "Database provisioning queued",
			Detail: created.Database.Engine + " " + created.Database.DefaultVersion,
		})
		payload := map[string]any{
			"status": "queued", "service": created.Service, "database": created.Database,
			"instances": created.Instances, "bindings": created.Bindings, "operations": created.Operations,
		}
		if len(initialPublicAccess) > 0 {
			payload["initial_external_connections"] = initialPublicAccess
		}
		if len(initialPublicAccessErrors) > 0 {
			payload["initial_external_connection_errors"] = initialPublicAccessErrors
		}
		writeJSON(w, http.StatusAccepted, payload)
		return
	}
	if len(parts) == 3 && parts[1] == "environments" && r.Method == http.MethodPatch {
		environment, err := s.store.GetEnvironment(r.Context(), parts[2])
		if err != nil || environment.ApplicationID != app.ID {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "environment_not_found"})
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "error": "invalid_environment_name", "fields": map[string]string{"name": "required"}})
			return
		}
		item, err := s.store.UpdateEnvironment(r.Context(), environment.ID, req.Name)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "update_environment_failed"})
			return
		}
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: app.ID, EnvironmentID: item.ID, EventType: "configuration", Status: "updated", Actor: "operator", Message: "Environment updated", Detail: item.Name})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "environment": item})
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			envs, envErr := s.store.ListApplicationEnvironments(r.Context(), app.ID)
			services, serviceErr := s.store.ListApplicationServices(r.Context(), app.ID)
			if envErr != nil || serviceErr != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "application_summary_failed"})
				return
			}
			bindings := make(map[string][]repository.ServiceEnvironment, len(services))
			databaseInstances := make(map[string][]repository.DatabaseInstance)
			for _, service := range services {
				if service.ServiceType == "database" {
					items, err := s.store.ListDatabaseInstances(r.Context(), service.ID)
					if err != nil {
						writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "application_summary_failed"})
						return
					}
					bindings[service.ID] = []repository.ServiceEnvironment{}
					databaseInstances[service.ID] = items
					continue
				}
				items, err := s.store.ListServiceEnvironments(r.Context(), service.ID)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "application_summary_failed"})
					return
				}
				bindings[service.ID] = items
			}
			writeJSON(w, http.StatusOK, map[string]any{"application": app, "environments": envs, "services": services, "service_bindings": bindings, "database_instances": databaseInstances})
		case http.MethodPatch:
			var req struct {
				Name        *string `json:"name"`
				Description *string `json:"description"`
				Archived    *bool   `json:"archived"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
				return
			}
			name, description := app.Name, app.Description
			if req.Name != nil {
				name = *req.Name
			}
			if req.Description != nil {
				description = *req.Description
			}
			item, err := s.store.UpdateApplication(r.Context(), app.ID, name, description, req.Archived)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "update_application_failed"})
				return
			}
			_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: item.ID, EventType: "application", Status: "updated", Actor: "operator", Message: "Application updated", Detail: item.Name})
			writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "application": item})
		case http.MethodDelete:
			result, err := platformservices.DeleteApplicationAndRuntime(r.Context(), s.log, s.cfg, s.store, s.dockerClient, app.ID)
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": publicAPIError(err, "delete_application_failed")})
				return
			}
			response := map[string]any{"status": "deleted"}
			eventStatus, eventDetail := "deleted", app.Name
			if result.CaddySyncError != "" {
				response["routing_warning"] = result.CaddySyncError
				eventStatus = "warning"
				eventDetail += "; routing cleanup: " + result.CaddySyncError
			}
			_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{EventType: "application", Status: eventStatus, Actor: "operator", Message: "Application deleted", Detail: eventDetail})
			writeJSON(w, http.StatusOK, response)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	if len(parts) == 2 && parts[1] == "environments" && r.Method == http.MethodGet {
		items, err := s.store.ListApplicationEnvironments(r.Context(), app.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_environments_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"environments": items})
		return
	}
	if len(parts) == 2 && parts[1] == "environments" && r.Method == http.MethodPost {
		var req struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
			Kind string `json:"kind"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
			return
		}
		item, err := s.store.CreateEnvironment(r.Context(), app.ID, req.Name, req.Slug, req.Kind)
		if err != nil {
			status, code := http.StatusBadRequest, "create_environment_failed"
			if err == repository.ErrDuplicateEnvironment {
				status, code = http.StatusConflict, "duplicate_environment"
			}
			writeJSON(w, status, map[string]string{"status": "error", "error": code})
			return
		}
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: app.ID, EnvironmentID: item.ID, EventType: "configuration", Status: "created", Actor: "operator", Message: "Environment created", Detail: item.Name})
		writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "environment": item})
		return
	}
	if len(parts) == 2 && parts[1] == "services" && r.Method == http.MethodGet {
		items, err := s.store.ListApplicationServices(r.Context(), app.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_services_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"services": items})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
}
