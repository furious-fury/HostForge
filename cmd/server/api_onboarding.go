package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/caddy"
	"github.com/hostforge/hostforge/internal/dnsops"
	"github.com/hostforge/hostforge/internal/repository"
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
	timeout := time.Duration(s.cfg.DNSDetectTimeoutMS) * time.Millisecond
	status, _ := dnsops.CheckRegistrarARecord(r.Context(), domain, s.cfg.BootstrapPublicIP, timeout)
	if status != "ok" {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "platform_dns_not_ready"})
		return
	}
	if strings.TrimSpace(s.cfg.CaddyRootConfig) == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "error", "error": "caddy_root_config_required"})
		return
	}
	if err := caddy.ReplaceRoot(r.Context(), s.cfg.CaddyBin, s.cfg.CaddyRootConfig, caddy.RenderPermanentControlPlaneConfig(domain, s.cfg.CaddyGeneratedPath)); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "permanent_https_provision_failed"})
		return
	}
	if err := s.store.CompleteOnboarding(r.Context(), domain); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "onboarding_completion_failed"})
		return
	}
	_ = s.store.RecordPlatformEvent(r.Context(), repository.PlatformEventInput{EventType: "configuration", Status: "completed", Actor: "operator", Message: "Onboarding completed", Detail: domain})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "bootstrap_disabled": true, "platform_domain": domain})
}
