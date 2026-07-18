package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/dnsops"
	"github.com/hostforge/hostforge/internal/repository"
	platformservices "github.com/hostforge/hostforge/internal/services"
)

func (s *server) handleDatabaseGateways(w http.ResponseWriter, r *http.Request) {
	engine := strings.ToLower(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/database-gateways/"), "/"))
	if engine == "" || strings.Contains(engine, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	registry := platformservices.NewDatabaseGatewayAdapterRegistry()
	adapterAvailable := registry.Available(engine)
	if r.Method == http.MethodGet {
		response := map[string]any{"engine": engine, "feature_enabled": s.cfg.DatabaseGatewaysEnabled, "adapter_available": adapterAvailable}
		if endpoint, err := s.store.GetDatabaseGatewayEndpoint(r.Context(), engine); err == nil {
			response["gateway"] = endpoint
		} else {
			response["gateway"] = nil
		}
		if state, err := s.store.GetOnboardingState(r.Context()); err == nil && strings.TrimSpace(state.PlatformDomain) != "" && adapterAvailable {
			adapter, _ := registry.Get(engine)
			if endpoint, endpointErr := adapter.Endpoint(r.Context(), platformservices.GatewayEndpointRequest{PlatformDomain: state.PlatformDomain}); endpointErr == nil {
				response["reserved_hostname"] = endpoint.Hostname
			}
		}
		if expectedIPv4, _, _ := dnsops.ResolveExpectedIPv4(r.Context(), s.cfg); strings.TrimSpace(expectedIPv4) != "" {
			response["expected_ipv4"] = expectedIPv4
		}
		if !adapterAvailable {
			response["unavailable_reason"] = "external_access_engine_unsupported"
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	// Keep gateway desired-state mutations outside the database/Caddy domain
	// transition window so rollback cannot be defeated by a newly queued grant.
	s.databaseGatewayDomainMu.RLock()
	defer s.databaseGatewayDomainMu.RUnlock()
	if !s.cfg.DatabaseGatewaysEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "database_gateways_disabled"})
		return
	}
	if !adapterAvailable {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "external_access_engine_unsupported"})
		return
	}
	switch r.Method {
	case http.MethodPost:
		endpoint, err := s.ensurePostgreSQLGatewayEndpoint(r)
		if err != nil {
			writeDatabaseGatewayError(w, err)
			return
		}
		operation, err := s.store.QueueDatabaseGatewayProvision(r.Context(), endpoint.Engine, "operator")
		if err != nil {
			writeDatabaseGatewayError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "operation": operation})
	case http.MethodDelete:
		var request struct {
			Confirmation string `json:"confirmation"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
			return
		}
		if request.Confirmation != "TEAR DOWN POSTGRESQL GATEWAY" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "gateway_teardown_confirmation_required"})
			return
		}
		operation, err := s.store.QueueDatabaseGatewayTeardown(r.Context(), engine, "operator")
		if err != nil {
			writeDatabaseGatewayError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "operation": operation})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
	}
}

func (s *server) ensurePostgreSQLGatewayEndpoint(r *http.Request) (repository.DatabaseGatewayEndpoint, error) {
	if err := platformservices.ValidatePgBouncerImage(s.cfg.PostgreSQLGatewayImage, s.cfg.PostgreSQLGatewayVersion); err != nil {
		return repository.DatabaseGatewayEndpoint{}, platformservices.ErrCode("database_gateway_image_unavailable", err)
	}
	state, err := s.store.GetOnboardingState(r.Context())
	if err != nil || strings.TrimSpace(state.PlatformDomain) == "" {
		return repository.DatabaseGatewayEndpoint{}, platformservices.ErrCode("database_gateway_platform_domain_required", errors.New("platform domain is required"))
	}
	adapter, err := platformservices.NewDatabaseGatewayAdapterRegistry().Get("postgresql")
	if err != nil {
		return repository.DatabaseGatewayEndpoint{}, err
	}
	endpointSpec, err := adapter.Endpoint(r.Context(), platformservices.GatewayEndpointRequest{PlatformDomain: state.PlatformDomain})
	if err != nil {
		return repository.DatabaseGatewayEndpoint{}, err
	}
	endpoint, err := s.store.EnsureDatabaseGatewayEndpointForPlatformDomain(r.Context(), endpointSpec.Engine, endpointSpec.Hostname, s.cfg.PostgreSQLGatewayImage, s.cfg.PostgreSQLGatewayVersion, state.PlatformDomain)
	if errors.Is(err, repository.ErrPlatformDomainChanged) {
		return repository.DatabaseGatewayEndpoint{}, platformservices.ErrCode("database_gateway_platform_domain_changed", err)
	}
	if err != nil {
		return repository.DatabaseGatewayEndpoint{}, err
	}
	return endpoint, nil
}

func (s *server) handleDatabaseGatewayOperations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/database-gateway-operations/"), "/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	operation, err := s.store.GetDatabaseGatewayOperation(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "database_gateway_operation_not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "database_gateway_operation_lookup_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"operation": operation})
}

func (s *server) handleDatabaseInstanceExternalAccess(w http.ResponseWriter, r *http.Request, instanceID string) {
	access, err := s.store.GetDatabaseExternalAccess(r.Context(), instanceID)
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	databaseService, err := s.store.GetDatabaseService(r.Context(), access.Instance.ServiceID)
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	response := map[string]any{
		"feature_enabled":   s.cfg.DatabaseGatewaysEnabled,
		"adapter_available": platformservices.NewDatabaseGatewayAdapterRegistry().Available(databaseService.Engine),
		"engine":            databaseService.Engine,
		"external_access":   access,
	}
	if clientIP := net.ParseIP(requestIP(r)); clientIP != nil && !clientIP.IsLoopback() && !clientIP.IsUnspecified() {
		if clientIP.To4() != nil {
			response["client_ip"] = clientIP.String() + "/32"
		} else {
			response["client_ip"] = clientIP.String() + "/128"
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *server) handleDatabaseInstanceExternalConnections(w http.ResponseWriter, r *http.Request, instanceID string) {
	s.databaseGatewayDomainMu.RLock()
	defer s.databaseGatewayDomainMu.RUnlock()
	if !s.cfg.DatabaseGatewaysEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "database_gateways_disabled"})
		return
	}
	instance, err := s.store.GetDatabaseInstance(r.Context(), instanceID)
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	databaseService, err := s.store.GetDatabaseService(r.Context(), instance.ServiceID)
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	if databaseService.Engine != "postgresql" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "external_access_engine_unsupported"})
		return
	}
	if _, err := s.ensurePostgreSQLGatewayEndpoint(r); err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	var request struct {
		Name              string   `json:"name"`
		Profile           string   `json:"profile"`
		CIDRs             []string `json:"cidrs"`
		ExpiresAt         string   `json:"expires_at"`
		ConfirmOpenAccess bool     `json:"confirm_open_access"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	var expiresAt time.Time
	if strings.TrimSpace(request.ExpiresAt) != "" {
		expiresAt, err = time.Parse(time.RFC3339, request.ExpiresAt)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "invalid_external_access_expiry"})
			return
		}
	}
	connection, operation, err := s.store.CreateDatabaseExternalConnection(r.Context(), instanceID, repository.CreateExternalConnectionInput{Name: request.Name, PermissionProfile: request.Profile, CIDRs: request.CIDRs, ExpiresAt: expiresAt, ConfirmOpenAccess: request.ConfirmOpenAccess, Actor: "operator"})
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	s.recordDatabaseGatewayEvent(r, instance, "queued", "Database external connection creation queued", operation.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "connection": connection, "operation": operation})
}

func (s *server) handleDatabaseExternalConnections(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/database-external-connections/"), "/"), "/")
	if len(parts) < 1 || len(parts) > 2 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	if !s.cfg.DatabaseGatewaysEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "database_gateways_disabled"})
		return
	}
	s.databaseGatewayDomainMu.RLock()
	defer s.databaseGatewayDomainMu.RUnlock()
	connectionID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodPatch {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
			return
		}
		s.handleDatabaseExternalConnectionUpdate(w, r, connectionID)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	action := strings.ToLower(parts[1])
	if action == "credentials" {
		s.handleDatabaseExternalConnectionCredentials(w, r, connectionID)
		return
	}
	if action != "disable" && action != "enable" && action != "rotate" && action != "revoke" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "route_not_found"})
		return
	}
	var request struct {
		GracePeriodHours *int   `json:"grace_period_hours"`
		Confirmation     string `json:"confirmation"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
			return
		}
	}
	if action == "revoke" && request.Confirmation != "REVOKE EXTERNAL CONNECTION" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "external_connection_revoke_confirmation_required"})
		return
	}
	graceHours := repository.DefaultRotationGracePeriodHours
	if request.GracePeriodHours != nil {
		graceHours = *request.GracePeriodHours
	}
	operation, err := s.store.QueueDatabaseExternalConnectionAction(r.Context(), connectionID, action, graceHours, "operator")
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "operation": operation})
}

func (s *server) handleDatabaseExternalConnectionUpdate(w http.ResponseWriter, r *http.Request, connectionID string) {
	var request struct {
		Name              string   `json:"name"`
		Profile           string   `json:"profile"`
		CIDRs             []string `json:"cidrs"`
		ExpiresAt         *string  `json:"expires_at"`
		ConfirmOpenAccess bool     `json:"confirm_open_access"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	input := repository.UpdateExternalConnectionInput{Name: request.Name, PermissionProfile: request.Profile, CIDRs: request.CIDRs, ConfirmOpenAccess: request.ConfirmOpenAccess, Actor: "operator"}
	if request.ExpiresAt != nil {
		if strings.TrimSpace(*request.ExpiresAt) == "" {
			input.ClearExpiry = true
		} else {
			expiresAt, err := time.Parse(time.RFC3339, *request.ExpiresAt)
			if err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "error", "error": "invalid_external_access_expiry"})
				return
			}
			input.ExpiresAt = expiresAt
		}
	}
	connection, operation, err := s.store.UpdateDatabaseExternalConnection(r.Context(), connectionID, input)
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "connection": connection, "operation": operation})
}

func (s *server) handleDatabaseExternalConnectionCredentials(w http.ResponseWriter, r *http.Request, connectionID string) {
	connection, err := s.store.GetDatabaseExternalConnection(r.Context(), connectionID)
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	if connection.Status != "active" && connection.Status != "rotating" {
		writeDatabaseGatewayError(w, repository.ErrInvalidExternalConnectionState)
		return
	}
	credentials, err := s.store.ListDatabaseExternalCredentials(r.Context(), connection.ID, true)
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	var credential *repository.DatabaseExternalCredential
	for index := range credentials {
		if credentials[index].Generation == connection.CurrentGeneration && credentials[index].State == "active" {
			credential = &credentials[index]
			break
		}
	}
	if credential == nil || s.envSealer == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "external_connection_credentials_unavailable"})
		return
	}
	password, err := s.envSealer.Open(credential.PasswordCT)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "external_connection_credentials_unavailable"})
		return
	}
	defer func() {
		for index := range password {
			password[index] = 0
		}
	}()
	route, err := s.store.GetDatabaseGatewayRoute(r.Context(), connection.RouteID)
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	endpoint, err := s.store.GetDatabaseGatewayEndpoint(r.Context(), route.Engine)
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	adapter := platformservices.NewPostgreSQLGatewayAdapter(nil)
	connectionURL, err := adapter.ConnectionURL(platformservices.ConnectionURLRequest{Username: credential.RoleName, Password: string(password), Hostname: endpoint.Hostname, Port: endpoint.Port, Alias: route.RouteAlias})
	if err != nil {
		writeDatabaseGatewayError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"username": credential.RoleName, "password": string(password), "database_alias": route.RouteAlias, "hostname": endpoint.Hostname, "port": endpoint.Port, "sslmode": "verify-full", "url": connectionURL, "generation": credential.Generation})
}

func (s *server) recordDatabaseGatewayEvent(r *http.Request, instance repository.DatabaseInstance, status, message, detail string) {
	service, err := s.store.GetService(r.Context(), instance.ServiceID)
	if err != nil {
		return
	}
	_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: service.ApplicationID, ServiceID: service.ID, EnvironmentID: instance.EnvironmentID, EventType: "database_external_access", Status: status, Actor: "operator", Message: message, Detail: detail})
}

func writeDatabaseGatewayError(w http.ResponseWriter, err error) {
	code, status := "database_gateway_operation_failed", http.StatusInternalServerError
	switch {
	case errors.Is(err, repository.ErrDatabaseInstanceNotFound), errors.Is(err, repository.ErrExternalConnectionNotFound), errors.Is(err, repository.ErrDatabaseGatewayRouteNotFound), errors.Is(err, repository.ErrDatabaseGatewayNotFound), errors.Is(err, sql.ErrNoRows):
		code, status = "database_gateway_resource_not_found", http.StatusNotFound
	case errors.Is(err, repository.ErrInvalidExternalAccessCIDR):
		code, status = "invalid_external_access_cidr", http.StatusUnprocessableEntity
	case errors.Is(err, repository.ErrInvalidExternalAccessProfile):
		code, status = "invalid_external_access_profile", http.StatusUnprocessableEntity
	case errors.Is(err, repository.ErrInvalidExternalAccessExpiry):
		code, status = "invalid_external_access_expiry", http.StatusUnprocessableEntity
	case errors.Is(err, repository.ErrOpenAccessConfirmationRequired):
		code, status = "external_access_open_confirmation_required", http.StatusUnprocessableEntity
	case errors.Is(err, repository.ErrInvalidExternalConnectionState):
		code, status = "invalid_external_connection_state", http.StatusConflict
	case errors.Is(err, repository.ErrGatewayHasActiveConnections):
		code, status = "database_gateway_has_active_connections", http.StatusConflict
	default:
		if public := platformservices.FirstPublicCode(err); public != "internal_error" {
			code = public
			status = http.StatusConflict
		}
	}
	writeJSON(w, status, map[string]string{"status": "error", "error": code})
}
