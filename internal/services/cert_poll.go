package services

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/obs"
	"github.com/furious-fury/HostForge/internal/redact"
	"github.com/furious-fury/HostForge/internal/repository"
)

const maxCertMessageLen = 512

// StartCaddyCertPollLoop runs PollCaddyCertObservations on an interval until ctx is
// cancelled. It is a no-op when cfg.CaddyCertPollIntervalSec <= 0.
// obsCtx should carry observability store (e.g. obs.WithStore(context.Background(), store)) for UI samples.
func StartCaddyCertPollLoop(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, obsCtx context.Context) {
	sec := cfg.CaddyCertPollIntervalSec
	if sec <= 0 {
		return
	}
	interval := time.Duration(sec) * time.Second
	log = log.With("component", "caddy_cert_poll")
	if log != nil {
		log.Info("caddy cert poll enabled", "interval_sec", sec, "admin_url", redact.HTTPURLForLog(cfg.CaddyAdminURL), "storage_root", cfg.CaddyStorageRoot)
	}
	if obsCtx == nil {
		obsCtx = context.Background()
	}
	run := func() {
		t0 := time.Now()
		if err := PollCaddyCertObservations(obsCtx, log, cfg, store); err != nil {
			log.Warn("cert poll tick failed", "duration_ms", time.Since(t0).Milliseconds(), "error", err)
			recordCertPollObs(obsCtx, log, t0, "failed", err)
			return
		}
		recordCertPollObs(obsCtx, log, t0, "ok", nil)
	}
	run()
	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
}

// PollCaddyCertObservations updates last_cert_message / cert_checked_at for
// each domain row, and now owns ssl_status too: it is the only signal in the
// codebase that actually inspects a certificate, so it is the only thing
// that should say whether one exists. This is a real split from
// publish_state (internal/reconcile/caddy), which answers a different
// question -- "is Caddy currently routing this domain" -- and is owned by
// the reconciler. A domain can be published with no certificate yet
// (ACME still pending) or have a valid certificate for a route that was
// since unpublished; the two fields are allowed to disagree.
func PollCaddyCertObservations(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store) error {
	tickStart := time.Now()
	domains, err := store.ListAllDomains(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	adminNote, adminErr := probeCaddyAdminConfig(ctx, cfg)
	if adminErr != nil && log != nil {
		log.Debug("caddy admin probe", "error", adminErr)
	}
	storageRoot := strings.TrimSpace(cfg.CaddyStorageRoot)
	certRoot := ""
	if storageRoot != "" {
		certRoot = filepath.Join(storageRoot, "certificates")
	}
	for _, d := range domains {
		observation := observeCertificateFile(certRoot, d.DomainName)
		msg := buildCertObservationMessage(storageRoot, observation, adminNote)
		msg = truncateStr(msg, maxCertMessageLen)
		if err := store.UpdateDomainCertObservation(ctx, d.ID, msg, now); err != nil && log != nil {
			log.Warn("update cert observation", "domain_id", d.ID, "error", err)
		}
		if storageRoot != "" {
			if err := store.UpdateDomainSSLStatus(ctx, d.ID, observation.sslStatus); err != nil && log != nil {
				log.Warn("update domain ssl status", "domain_id", d.ID, "status", observation.sslStatus, "error", err)
			}
		}
	}
	dur := time.Since(tickStart).Milliseconds()
	if log != nil {
		log.Info("cert_poll tick complete", "domain_count", len(domains), "duration_ms", dur)
	}
	return nil
}

func recordCertPollObs(ctx context.Context, log *slog.Logger, started time.Time, status string, pollErr error) {
	code := ""
	if pollErr != nil {
		code = "cert_poll_failed"
	}
	obs.RecordDeployStep(ctx, log, models.DeployStepRecord{
		DeploymentID:  "",
		ServiceID:     "",
		EnvironmentID: "",
		RequestID:     "",
		Step:          "cert_poll",
		Status:        status,
		DurationMS:    time.Since(started).Milliseconds(),
		ErrorCode:     code,
		StartedAt:     started.UTC(),
		EndedAt:       time.Now().UTC(),
	})
}

// certFileObservation is what inspecting one domain's on-disk certificate
// found: a human message for last_cert_message, and the ssl_status it
// implies. sslStatus is only meaningful when storageRoot was configured;
// PollCaddyCertObservations skips the ssl_status write otherwise.
type certFileObservation struct {
	message   string
	sslStatus string
}

func buildCertObservationMessage(storageRoot string, observation certFileObservation, adminNote string) string {
	var parts []string
	if storageRoot == "" {
		parts = append(parts, "storage: unset")
	} else {
		parts = append(parts, observation.message)
	}
	if strings.TrimSpace(adminNote) != "" {
		parts = append(parts, adminNote)
	}
	return strings.Join(parts, "; ")
}

// observeCertificateFile inspects the newest managed leaf certificate for
// domain under certRoot and reports both a diagnostic message and the
// ssl_status it implies. certRoot == "" (storage root unset) reports
// PENDING -- there is no signal either way, and PENDING is the row's
// pre-existing default, so this is a no-op status rather than a claim.
func observeCertificateFile(certRoot, domain string) certFileObservation {
	if certRoot == "" {
		return certFileObservation{message: "", sslStatus: models.SSLStatusPending}
	}
	pattern := filepath.Join(certRoot, "*", domain, domain+".crt")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return certFileObservation{message: "storage: no_managed_leaf_pem", sslStatus: models.SSLStatusPending}
	}
	var best string
	var bestMod time.Time
	for _, p := range matches {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if best == "" || st.ModTime().After(bestMod) {
			best, bestMod = p, st.ModTime()
		}
	}
	if best == "" {
		return certFileObservation{message: "storage: no_managed_leaf_pem", sslStatus: models.SSLStatusPending}
	}
	data, err := os.ReadFile(best)
	if err != nil {
		return certFileObservation{message: fmt.Sprintf("storage: read_err path=%s", filepath.Base(best)), sslStatus: models.SSLStatusError}
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return certFileObservation{message: "storage: invalid_pem", sslStatus: models.SSLStatusError}
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return certFileObservation{message: "storage: parse_cert_failed", sslStatus: models.SSLStatusError}
	}
	exp := cert.NotAfter.UTC().Format(time.RFC3339)
	iss := strings.TrimSpace(cert.Issuer.CommonName)
	if iss == "" {
		iss = "unknown_issuer"
	}
	msg := fmt.Sprintf("leaf_expires=%s issuer=%s", exp, iss)
	if time.Now().After(cert.NotAfter) {
		return certFileObservation{message: msg + " expired=true", sslStatus: models.SSLStatusError}
	}
	if time.Until(cert.NotAfter) < 14*24*time.Hour {
		msg += " expiring_soon=true"
	}
	return certFileObservation{message: msg, sslStatus: models.SSLStatusActive}
}

func probeCaddyAdminConfig(ctx context.Context, cfg *config.Config) (string, error) {
	base := strings.TrimSpace(cfg.CaddyAdminURL)
	if base == "" {
		return "", nil
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid caddy admin url")
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/config/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "admin: unreachable", err
	}
	defer resp.Body.Close()
	peek, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("admin: http_%d", resp.StatusCode), nil
	}
	s := strings.TrimSpace(string(peek))
	if s == "" || s == "{}" {
		return "admin: empty_config", nil
	}
	return "admin: config_present", nil
}

func truncateStr(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
