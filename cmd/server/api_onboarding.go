package main

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/caddy"
	"github.com/furious-fury/HostForge/internal/dnsops"
	"github.com/furious-fury/HostForge/internal/repository"
	platformservices "github.com/furious-fury/HostForge/internal/services"
)

type onboardingCompleteRequest struct {
	PlatformDomain string `json:"platform_domain"`
}

func (s *server) handleOnboardingRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		state, err := s.store.GetOnboardingState(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "onboarding_state_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"onboarding": map[string]any{
			"bootstrap_enabled":   s.cfg.BootstrapEnabled && !state.BootstrapComplete,
			"bootstrap_public_ip": s.cfg.BootstrapPublicIP, "bootstrap_https_port": s.cfg.BootstrapHTTPSPort,
			"bootstrap_expires_at": s.cfg.BootstrapExpiresAt, "github_app_complete": state.GitHubAppComplete,
			"platform_domain": state.PlatformDomain, "permanent_ingress_complete": state.PermanentIngressComplete,
			"bootstrap_complete": state.BootstrapComplete, "completed_at": state.CompletedAt,
		}})
	case http.MethodPatch:
		s.handleOnboardingComplete(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
	}
}

func (s *server) handleOnboardingComplete(w http.ResponseWriter, r *http.Request) {
	var in onboardingCompleteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json"})
		return
	}
	domain := strings.ToLower(strings.TrimSpace(in.PlatformDomain))
	if err := dnsops.ValidateDomainName(domain); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_platform_domain"})
		return
	}
	state, err := s.store.GetOnboardingState(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "onboarding_state_failed"})
		return
	}
	if state.BootstrapComplete {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "bootstrap_already_complete"})
		return
	}
	if !state.GitHubAppComplete {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "github_app_incomplete"})
		return
	}
	expectedIPv4 := strings.TrimSpace(s.cfg.DNSServerIPv4)
	if parsed := net.ParseIP(expectedIPv4); parsed == nil || parsed.To4() == nil {
		expectedIPv4 = strings.TrimSpace(s.cfg.BootstrapPublicIP)
	}
	if parsed := net.ParseIP(expectedIPv4); parsed == nil || parsed.To4() == nil {
		expectedIPv4, _, _ = dnsops.ResolveExpectedIPv4(r.Context(), s.cfg)
	}
	if expectedIPv4 == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "expected_public_ipv4_unavailable"})
		return
	}
	timeout := time.Duration(s.cfg.DNSDetectTimeoutMS) * time.Millisecond
	wildcardProbe := "hostforge-wildcard-check." + domain
	checks := dnsops.CheckRegistrarARecords(r.Context(), []string{domain, wildcardProbe}, expectedIPv4, timeout)
	apexStatus, wildcardStatus := "lookup_error", "lookup_error"
	resolved := map[string][]string{"apex": {}, "wildcard": {}}
	if len(checks) > 0 {
		apexStatus = checks[0].Status
		resolved["apex"] = checks[0].ResolvedIPv4
	}
	if len(checks) > 1 {
		wildcardStatus = checks[1].Status
		resolved["wildcard"] = checks[1].ResolvedIPv4
	}
	if apexStatus != "ok" || wildcardStatus != "ok" {
		writeJSON(w, http.StatusConflict, map[string]any{
			"status": "error", "error": "platform_dns_not_ready",
			"expected_ipv4": expectedIPv4,
			"hostnames":     map[string]string{"apex": domain, "wildcard": wildcardProbe},
			"checks":        map[string]string{"apex": apexStatus, "wildcard": wildcardStatus},
			"resolved_ipv4": resolved,
		})
		return
	}
	if strings.TrimSpace(s.cfg.CaddyRootConfig) == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "caddy_root_config_required"})
		return
	}
	if err := caddy.ReplaceManagedConfig(r.Context(), s.cfg.CaddyBin, s.cfg.CaddyControlPlanePath, s.cfg.CaddyRootConfig, caddy.RenderPermanentControlPlaneConfig(domain)); err != nil {
		s.requestLog(r).Error("permanent control-plane caddy update failed", "domain", domain, "managed_path", s.cfg.CaddyControlPlanePath, "root_config", s.cfg.CaddyRootConfig, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "permanent_https_provision_failed", "message": "Caddy could not apply the permanent platform route. Run the VPS update to migrate the managed Caddy layout, then retry."})
		return
	}
	if err := s.store.CompleteOnboarding(r.Context(), domain); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "onboarding_completion_failed"})
		return
	}
	if created, provisionErr := s.store.EnsureActivePlatformServiceDomains(r.Context()); provisionErr != nil {
		s.requestLog(r).Warn("provision existing platform share domains failed", "error", provisionErr)
	} else if created > 0 {
		if syncErr := platformservices.SyncCaddyRoutes(r.Context(), s.requestLog(r), s.cfg, s.store); syncErr != nil {
			s.requestLog(r).Warn("sync existing platform share domains failed", "created", created, "error", syncErr)
		}
	}
	_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{EventType: "configuration", Status: "completed", Actor: "operator", Message: "Onboarding completed", Detail: domain})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "bootstrap_disabled": true, "platform_domain": domain})
}
