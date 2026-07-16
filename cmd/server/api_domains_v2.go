package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/dnsops"
	"github.com/hostforge/hostforge/internal/repository"
)

type serviceDomainRequest struct {
	DomainName string `json:"domain_name"`
	ServiceID  string `json:"service_id"`
}

func (s *server) handleServiceDomains(w http.ResponseWriter, r *http.Request, applicationID, environmentID, domainID string) {
	environment, err := s.store.GetEnvironment(r.Context(), environmentID)
	if err != nil || environment.ApplicationID != applicationID {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "environment_not_found"})
		return
	}
	if domainID == "" {
		switch r.Method {
		case http.MethodGet:
			serviceID := strings.TrimSpace(r.URL.Query().Get("service_id"))
			if serviceID != "" {
				service, err := s.store.GetService(r.Context(), serviceID)
				if err != nil || service.ApplicationID != applicationID {
					writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_service_scope"})
					return
				}
			}
			items, err := s.store.ListServiceDomains(r.Context(), applicationID, environmentID, serviceID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_domains_failed"})
				return
			}
			names := make([]string, 0, len(items))
			for _, item := range items {
				names = append(names, item.DomainName)
			}
			expectedIPv4, source, warning := dnsops.ResolveExpectedIPv4(r.Context(), s.cfg)
			guidance := dnsops.BuildGuidanceWithIPv4(r.Context(), s.cfg, names, expectedIPv4, source, warning)
			if r.URL.Query().Get("check_dns") == "1" || strings.EqualFold(r.URL.Query().Get("check_dns"), "true") {
				timeout := time.Duration(s.cfg.DNSDetectTimeoutMS) * time.Millisecond
				guidance.Checks = dnsops.CheckRegistrarARecords(r.Context(), names, expectedIPv4, timeout)
			}
			writeJSON(w, http.StatusOK, map[string]any{"domains": items, "dns_guidance": guidance})
		case http.MethodPost:
			var req serviceDomainRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
				return
			}
			if err := dnsops.ValidateDomainName(req.DomainName); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": mapDomainValidationError(err)})
				return
			}
			item, err := s.store.CreateServiceDomain(r.Context(), applicationID, environmentID, req.ServiceID, req.DomainName)
			if errors.Is(err, repository.ErrEnvironmentNotFound) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "service_environment_not_found"})
				return
			}
			if errors.Is(err, repository.ErrDuplicateDomain) {
				writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "duplicate_domain"})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "create_domain_failed"})
				return
			}
			sync := s.caddySyncAfterDomainChange(r.Context(), s.requestLog(r))
			if sync.Attempted && !sync.OK {
				_ = s.store.DeleteServiceDomain(r.Context(), applicationID, environmentID, item.ID)
				_ = s.caddySyncAfterDomainChange(r.Context(), s.requestLog(r))
				writeJSON(w, http.StatusBadGateway, map[string]any{"status": "error", "error": "caddy_sync_failed", "caddy_sync": sync})
				return
			}
			_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: applicationID, ServiceID: item.ServiceID, EnvironmentID: environmentID, EventType: "domain", Status: "created", Actor: "operator", Message: "Domain added", Detail: item.DomainName})
			writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "domain": item, "caddy_sync": sync})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		}
		return
	}
	existing, err := s.store.GetServiceDomain(r.Context(), applicationID, environmentID, domainID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "domain_not_found"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req serviceDomainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
			return
		}
		if err := dnsops.ValidateDomainName(req.DomainName); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": mapDomainValidationError(err)})
			return
		}
		item, err := s.store.UpdateServiceDomain(r.Context(), applicationID, environmentID, domainID, req.DomainName, req.ServiceID)
		if errors.Is(err, repository.ErrDuplicateDomain) {
			writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "duplicate_domain"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "update_domain_failed"})
			return
		}
		sync := s.caddySyncAfterDomainChange(r.Context(), s.requestLog(r))
		if sync.Attempted && !sync.OK {
			_, _ = s.store.UpdateServiceDomain(r.Context(), applicationID, environmentID, domainID, existing.DomainName, existing.ServiceID)
			_ = s.caddySyncAfterDomainChange(r.Context(), s.requestLog(r))
			writeJSON(w, http.StatusBadGateway, map[string]any{"status": "error", "error": "caddy_sync_failed", "caddy_sync": sync})
			return
		}
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: applicationID, ServiceID: item.ServiceID, EnvironmentID: environmentID, EventType: "domain", Status: "updated", Actor: "operator", Message: "Domain updated", Detail: item.DomainName})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "domain": item, "caddy_sync": sync})
	case http.MethodDelete:
		if err := s.store.DeleteServiceDomain(r.Context(), applicationID, environmentID, domainID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "delete_domain_failed"})
			return
		}
		sync := s.caddySyncAfterDomainChange(r.Context(), s.requestLog(r))
		if sync.Attempted && !sync.OK {
			if restoreErr := s.store.RestoreServiceDomain(r.Context(), existing); restoreErr != nil {
				s.requestLog(r).Error("restore domain after caddy sync failure", "domain_id", existing.ID, "error", restoreErr)
			}
			_ = s.caddySyncAfterDomainChange(r.Context(), s.requestLog(r))
			writeJSON(w, http.StatusBadGateway, map[string]any{"status": "error", "error": "caddy_sync_failed", "caddy_sync": sync})
			return
		}
		_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{ApplicationID: applicationID, ServiceID: existing.ServiceID, EnvironmentID: environmentID, EventType: "domain", Status: "deleted", Actor: "operator", Message: "Domain removed", Detail: existing.DomainName})
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "domain_id": domainID, "caddy_sync": sync})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
	}
}
