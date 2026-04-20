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

	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/models"
	"github.com/hostforge/hostforge/internal/obs"
	"github.com/hostforge/hostforge/internal/redact"
	"github.com/hostforge/hostforge/internal/repository"
)

const maxCertMessageLen = 512

// StartCaddyCertPollLoop runs PollCaddyCertObservations on an interval until the process exits.
// It is a no-op when cfg.CaddyCertPollIntervalSec <= 0.
// obsCtx should carry observability store (e.g. obs.WithStore(context.Background(), store)) for UI samples.
func StartCaddyCertPollLoop(log *slog.Logger, cfg *config.Config, store *repository.Store, obsCtx context.Context) {
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
		for range t.C {
			run()
		}
	}()
}

// PollCaddyCertObservations updates last_cert_message / cert_checked_at for each domain row.
// It never changes ssl_status (route sync remains separate).
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
		msg := buildCertObservationMessage(storageRoot, certRoot, d.DomainName, adminNote)
		msg = truncateStr(msg, maxCertMessageLen)
		if err := store.UpdateDomainCertObservation(ctx, d.ID, msg, now); err != nil && log != nil {
			log.Warn("update cert observation", "domain_id", d.ID, "error", err)
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
		DeploymentID: "",
		ProjectID:    "",
		RequestID:    "",
		Step:         "cert_poll",
		Status:       status,
		DurationMS:   time.Since(started).Milliseconds(),
		ErrorCode:    code,
		StartedAt:    started.UTC(),
		EndedAt:      time.Now().UTC(),
	})
}

func buildCertObservationMessage(storageRoot, certRoot, domainName, adminNote string) string {
	var parts []string
	if storageRoot == "" {
		parts = append(parts, "storage: unset")
	} else if certRoot != "" {
		if m, ok := summarizeManagedCertFile(certRoot, domainName); ok {
			parts = append(parts, m)
		} else {
			parts = append(parts, "storage: no_managed_leaf_pem")
		}
	}
	if strings.TrimSpace(adminNote) != "" {
		parts = append(parts, adminNote)
	}
	return strings.Join(parts, "; ")
}

func summarizeManagedCertFile(certRoot, domain string) (string, bool) {
	pattern := filepath.Join(certRoot, "*", domain, domain+".crt")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", false
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
		return "", false
	}
	data, err := os.ReadFile(best)
	if err != nil {
		return fmt.Sprintf("storage: read_err path=%s", filepath.Base(best)), true
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return "storage: invalid_pem", true
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "storage: parse_cert_failed", true
	}
	exp := cert.NotAfter.UTC().Format(time.RFC3339)
	iss := strings.TrimSpace(cert.Issuer.CommonName)
	if iss == "" {
		iss = "unknown_issuer"
	}
	msg := fmt.Sprintf("leaf_expires=%s issuer=%s", exp, iss)
	if time.Until(cert.NotAfter) < 14*24*time.Hour {
		msg += " expiring_soon=true"
	}
	return msg, true
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
