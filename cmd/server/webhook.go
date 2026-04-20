package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/hostforge/hostforge/internal/auth"
	"github.com/hostforge/hostforge/internal/repository"
)

// verifyWebhookSignature accepts either the App-scoped webhook secret (sealed
// in the github_app row) or the legacy cfg.WebhookSecret. This lets operators
// keep existing org-level webhooks working while migrating to the App.
func (s *server) verifyWebhookSignature(ctx context.Context, signature string, body []byte) bool {
	if s.envSealer != nil {
		sealed, err := s.store.GetGitHubAppSealed(ctx)
		if err == nil {
			if pt, err := s.envSealer.Open(sealed.WebhookSecretCT); err == nil {
				if secret := strings.TrimSpace(string(pt)); secret != "" {
					if auth.VerifyGitHubSignature(secret, signature, body) {
						return true
					}
				}
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			s.log.Warn("github app sealed load failed during webhook verify", "error", err)
		}
	}
	if legacy := strings.TrimSpace(s.cfg.WebhookSecret); legacy != "" {
		if auth.VerifyGitHubSignature(legacy, signature, body) {
			return true
		}
	}
	return false
}

// installationEvent is a minimal subset of the installation / installation_repositories payloads.
type installationEvent struct {
	Action       string `json:"action"`
	Installation struct {
		ID            int64  `json:"id"`
		TargetType    string `json:"target_type"`
		RepoSelection string `json:"repository_selection"`
		SuspendedAt   string `json:"suspended_at"`
		Account       struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
	} `json:"installation"`
}

// handleInstallationEvent reflects installation lifecycle changes into the
// github_app_installations table so UI picks them up immediately without
// requiring a manual sync.
func (s *server) handleInstallationEvent(ctx context.Context, log *slog.Logger, eventType string, body []byte) {
	var evt installationEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		log.Warn("installation webhook decode failed", "event", eventType, "error", err)
		return
	}
	if evt.Installation.ID <= 0 {
		return
	}
	switch strings.TrimSpace(evt.Action) {
	case "deleted", "removed":
		if err := s.store.DeleteGitHubInstallation(ctx, evt.Installation.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Warn("delete installation failed", "installation_id", evt.Installation.ID, "error", err)
		}
		return
	case "suspend":
		if err := s.store.SuspendGitHubInstallation(ctx, evt.Installation.ID, true); err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Warn("suspend installation failed", "installation_id", evt.Installation.ID, "error", err)
		}
		return
	case "unsuspend":
		if err := s.store.SuspendGitHubInstallation(ctx, evt.Installation.ID, false); err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Warn("unsuspend installation failed", "installation_id", evt.Installation.ID, "error", err)
		}
		return
	}
	if err := s.store.UpsertGitHubInstallation(ctx, repository.UpsertGitHubInstallationInput{
		InstallationID: evt.Installation.ID,
		AccountLogin:   evt.Installation.Account.Login,
		AccountType:    evt.Installation.Account.Type,
		TargetType:     evt.Installation.TargetType,
		RepoSelection:  evt.Installation.RepoSelection,
		Suspended:      strings.TrimSpace(evt.Installation.SuspendedAt) != "",
	}); err != nil {
		log.Warn("upsert installation failed", "installation_id", evt.Installation.ID, "error", err)
	}
}
