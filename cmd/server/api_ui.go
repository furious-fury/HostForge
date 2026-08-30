package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/dnsops"
	"github.com/furious-fury/HostForge/internal/git"
	"github.com/furious-fury/HostForge/internal/services"
)

type caddySyncOutcome struct {
	Attempted bool   `json:"attempted"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

type repositoryBranchesResponse struct {
	Status        string   `json:"status"`
	RepoURL       string   `json:"repo_url"`
	Branches      []string `json:"branches"`
	DefaultBranch string   `json:"default_branch"`
}

func mapDomainValidationError(err error) string {
	switch {
	case errors.Is(err, dnsops.ErrDomainNameEmpty):
		return "domain_name_empty"
	case errors.Is(err, dnsops.ErrDomainNameTooLong):
		return "domain_name_too_long"
	case errors.Is(err, dnsops.ErrDomainNameInvalid):
		return "domain_name_invalid"
	default:
		return "invalid_domain_name"
	}
}

func (s *server) handleRepositoryBranches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	repoRaw := strings.TrimSpace(r.URL.Query().Get("repo_url"))
	if repoRaw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "missing_repo_url"})
		return
	}
	repoURL, err := services.CanonicalRepoURL(repoRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_repository_clone_url"})
		return
	}

	gitAuth := git.AuthOptions{}
	if raw := strings.TrimSpace(r.URL.Query().Get("installation_id")); raw != "" {
		installationID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || installationID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_installation_id"})
			return
		}
		gitAuth, err = s.resolveGitAuthForInstallation(r.Context(), installationID)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "installation_token_mint_failed"})
			return
		}
	}

	branches, inferredDefault, err := git.ListRemoteBranches(r.Context(), repoURL, gitAuth)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "list_remote_branches_failed"})
		return
	}
	defaultBranch := git.ResolveBranch(r.Context(), repoURL, "", gitAuth)
	writeJSON(w, http.StatusOK, repositoryBranchesResponse{
		Status: "ok", RepoURL: repoURL, Branches: branches,
		DefaultBranch: firstNonEmpty(inferredDefault, defaultBranch),
	})
}

func (s *server) caddySyncAfterDomainChange(ctx context.Context, lg *slog.Logger) caddySyncOutcome {
	out := caddySyncOutcome{}
	if !s.cfg.DomainSyncAfterMutate || strings.TrimSpace(s.cfg.CaddyRootConfig) == "" {
		return out
	}
	out.Attempted = true
	if lg == nil {
		lg = s.log
	}
	started := time.Now()
	if err := services.SyncCaddyRoutes(ctx, lg, s.cfg, s.store); err != nil {
		out.Error = publicAPIError(err, "caddy_sync_failed")
		lg.Warn("caddy sync after domain change failed", "error", err, "public_code", out.Error, "duration_ms", time.Since(started).Milliseconds())
		return out
	}
	out.OK = true
	lg.Info("caddy sync after domain change complete", "duration_ms", time.Since(started).Milliseconds())
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
