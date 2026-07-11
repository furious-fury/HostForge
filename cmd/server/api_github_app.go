package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	githubapp "github.com/hostforge/hostforge/internal/github/app"
	"github.com/hostforge/hostforge/internal/models"
	"github.com/hostforge/hostforge/internal/repository"
)

// isPublicWebhookURL reports whether a URL appears reachable over the public
// Internet. GitHub's App manifest validator rejects loopback / private
// addresses, and failing that check blocks the entire App creation flow.
func isPublicWebhookURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") ||
		lower == "0.0.0.0" || lower == "::" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return false
		}
	}
	return true
}

// apiGitHubApp is the non-sensitive App config returned to the UI.
type apiGitHubApp struct {
	Configured bool   `json:"configured"`
	AppID      int64  `json:"app_id,omitempty"`
	Slug       string `json:"slug,omitempty"`
	HTMLURL    string `json:"html_url,omitempty"`
	ClientID   string `json:"client_id,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// apiGitHubInstallation is one installation row for the UI.
type apiGitHubInstallation struct {
	InstallationID int64  `json:"installation_id"`
	AccountLogin   string `json:"account_login"`
	AccountType    string `json:"account_type"`
	TargetType     string `json:"target_type"`
	RepoSelection  string `json:"repo_selection"`
	Suspended      bool   `json:"suspended"`
	LastSyncedAt   string `json:"last_synced_at,omitempty"`
}

type apiGitHubRepo struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
}

// handleGitHubAppRoutes dispatches the /api/github/... tree.
func (s *server) handleGitHubAppRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/github/"), "/")
	parts := strings.Split(path, "/")
	switch {
	case path == "app":
		s.handleGitHubApp(w, r)
	case path == "app/manifest":
		s.handleGitHubAppManifest(w, r)
	case path == "app/exchange":
		s.handleGitHubAppExchange(w, r)
	case path == "installations":
		s.handleGitHubInstallations(w, r)
	case path == "installations/sync":
		s.handleGitHubInstallationsSync(w, r)
	case len(parts) == 3 && parts[0] == "installations" && parts[2] == "repositories":
		idRaw := parts[1]
		s.handleGitHubInstallationRepositories(w, r, idRaw)
	default:
		http.NotFound(w, r)
	}
}

func (s *server) handleGitHubApp(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		meta, err := s.store.GetGitHubAppMeta(r.Context())
		if err != nil {
			if errorsIsNoRows(err) {
				writeJSON(w, http.StatusOK, map[string]any{"app": apiGitHubApp{Configured: false}})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "app_lookup_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"app": apiGitHubApp{
			Configured: true,
			AppID:      meta.AppID,
			Slug:       meta.Slug,
			HTMLURL:    meta.HTMLURL,
			ClientID:   meta.ClientID,
			UpdatedAt:  formatTime(meta.UpdatedAt),
		}})
	case http.MethodDelete:
		if err := s.store.DeleteGitHubApp(r.Context()); err != nil {
			if errorsIsNoRows(err) {
				writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "app_not_configured"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "app_delete_failed"})
			return
		}
		s.invalidateAppClient()
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
	}
}

// appManifestRequest is what the UI POSTs to build a manifest payload. The UI
// then auto-submits an HTML form to github.com/settings/apps/new with the
// resulting manifest JSON in a hidden input.
type appManifestRequest struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	CallbackURL string `json:"callback_url"`
	// WebhookURL overrides the computed hook URL. GitHub rejects manifests
	// with a localhost / private-range webhook URL, so for local dev the UI
	// should pass a public URL (e.g. an ngrok tunnel) here.
	WebhookURL string `json:"webhook_url,omitempty"`
	// Organization may be empty for a personal App; when set, the UI should
	// redirect to github.com/organizations/{org}/settings/apps/new instead.
	Organization string `json:"organization,omitempty"`
}

type appManifestResponse struct {
	Status      string         `json:"status"`
	Manifest    map[string]any `json:"manifest"`
	PostURL     string         `json:"post_url"`
	CallbackURL string         `json:"callback_url"`
	WebhookURL  string         `json:"webhook_url"`
	State       string         `json:"state"`
}

// handleGitHubAppManifest returns the App manifest JSON the UI should POST to
// GitHub on the user's behalf. The server does NOT contact GitHub here; it
// only computes sane defaults (name, webhook URL, permissions, events).
func (s *server) handleGitHubAppManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"status": "error", "error": "content_type_must_be_application_json"})
		return
	}
	defer r.Body.Close()
	var req appManifestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}

	baseURL := strings.TrimRight(strings.TrimSpace(req.URL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(inferBaseURL(r), "/")
	}
	if baseURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "base_url_required"})
		return
	}
	callback := strings.TrimSpace(req.CallbackURL)
	if callback == "" {
		callback = baseURL + "/settings/github-app/callback"
	}
	webhookPath := strings.TrimSpace(s.cfg.WebhookBasePath)
	if webhookPath == "" {
		webhookPath = "/hooks/github"
	}
	webhookURL := strings.TrimSpace(req.WebhookURL)
	if webhookURL == "" {
		webhookURL = strings.TrimRight(baseURL, "/") + webhookPath
	}
	// GitHub rejects manifests whose webhook URL is not reachable over the
	// public Internet. If the computed/overridden URL is local, mark the
	// webhook as inactive so the manifest still validates; users can
	// activate it later once they have a public URL.
	hookActive := isPublicWebhookURL(webhookURL)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "HostForge"
	}
	state := fmt.Sprintf("hf-%d", time.Now().UTC().UnixNano())

	hookAttributes := map[string]any{"url": webhookURL, "active": hookActive}
	// Only subscribe to events that require a permission we actually ask
	// for. `installation` / `installation_repositories` are App lifecycle
	// events that GitHub delivers automatically and are NOT valid values
	// for `default_events`.
	// setup_url is where GitHub redirects after the user installs the App.
	setupURL := callback
	manifest := map[string]any{
		"name":                name,
		"url":                 baseURL,
		"hook_attributes":     hookAttributes,
		"redirect_url":        callback,
		"callback_urls":       []string{callback},
		"setup_url":           setupURL,
		"setup_on_update":     true,
		"public":              false,
		"default_events":      []string{"push"},
		"default_permissions": map[string]string{"contents": "read", "metadata": "read"},
	}
	post := "https://github.com/settings/apps/new"
	if org := strings.TrimSpace(req.Organization); org != "" {
		post = fmt.Sprintf("https://github.com/organizations/%s/settings/apps/new", org)
	}
	writeJSON(w, http.StatusOK, appManifestResponse{
		Status:      "ok",
		Manifest:    manifest,
		PostURL:     post + "?state=" + state,
		CallbackURL: callback,
		WebhookURL:  webhookURL,
		State:       state,
	})
}

type appExchangeRequest struct {
	Code string `json:"code"`
}

// handleGitHubAppExchange redeems the one-time manifest code for App credentials
// and seals the row. Also seeds the installations table from the GitHub API.
func (s *server) handleGitHubAppExchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	if !s.requireEnvSealer(w) {
		return
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"status": "error", "error": "content_type_must_be_application_json"})
		return
	}
	defer r.Body.Close()
	var req appExchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_json_payload"})
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "manifest_code_required"})
		return
	}

	creds, err := githubapp.ExchangeManifestCode(r.Context(), "", code, nil)
	if err != nil {
		s.requestLog(r).Error("github app manifest exchange failed", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "manifest_exchange_failed"})
		return
	}
	pemBytes := []byte(creds.PEM)
	if _, err := githubapp.ParsePrivateKeyPEM(pemBytes); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "manifest_private_key_invalid"})
		return
	}

	clientSecretCT, err := s.envSealer.Seal([]byte(creds.ClientSecret))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "seal_failed"})
		return
	}
	privateKeyCT, err := s.envSealer.Seal(pemBytes)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "seal_failed"})
		return
	}
	webhookCT, err := s.envSealer.Seal([]byte(creds.WebhookSecret))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "seal_failed"})
		return
	}

	meta, err := s.store.UpsertGitHubApp(r.Context(), repository.UpsertGitHubAppInput{
		AppID:           creds.ID,
		Slug:            creds.Slug,
		HTMLURL:         creds.HTMLURL,
		ClientID:        creds.ClientID,
		ClientSecretCT:  clientSecretCT,
		PrivateKeyCT:    privateKeyCT,
		WebhookSecretCT: webhookCT,
	})
	if err != nil {
		s.requestLog(r).Error("github app upsert failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "app_upsert_failed"})
		return
	}
	s.invalidateAppClient()

	if synced, err := s.syncInstallationsFromAPI(r.Context()); err != nil {
		s.requestLog(r).Warn("installations sync after manifest exchange failed", "error", err)
	} else {
		s.requestLog(r).Info("installations sync after manifest exchange", "count", synced)
	}

	installURL := ""
	if meta.Slug != "" {
		installURL = fmt.Sprintf("https://github.com/apps/%s/installations/new", meta.Slug)
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "app": apiGitHubApp{
		Configured: true,
		AppID:      meta.AppID,
		Slug:       meta.Slug,
		HTMLURL:    meta.HTMLURL,
		ClientID:   meta.ClientID,
		UpdatedAt:  formatTime(meta.UpdatedAt),
	}, "install_url": installURL})
}

// handleGitHubInstallations returns installations known to HostForge (from DB).
func (s *server) handleGitHubInstallations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	s.writeInstallations(w, r)
}

// writeInstallations loads the installations and writes the JSON response.
// Split out from handleGitHubInstallations so that POST /sync can reuse it
// without tripping the GET-only method guard.
func (s *server) writeInstallations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListGitHubInstallations(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "installations_list_failed"})
		return
	}
	out := make([]apiGitHubInstallation, 0, len(rows))
	for _, row := range rows {
		out = append(out, apiGitHubInstallation{
			InstallationID: row.InstallationID,
			AccountLogin:   row.AccountLogin,
			AccountType:    row.AccountType,
			TargetType:     row.TargetType,
			RepoSelection:  row.RepoSelection,
			Suspended:      strings.TrimSpace(row.SuspendedAt) != "",
			LastSyncedAt:   row.LastSyncedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"installations": out})
}

// handleGitHubInstallationsSync re-fetches installations from the GitHub API
// and upserts them, then returns the fresh list.
func (s *server) handleGitHubInstallationsSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	if _, err := s.syncInstallationsFromAPI(r.Context()); err != nil {
		if errors.Is(err, repository.ErrGitHubAppNotConfigured) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "app_not_configured"})
			return
		}
		s.requestLog(r).Error("installations sync failed", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "installations_sync_failed"})
		return
	}
	s.writeInstallations(w, r)
}

// handleGitHubInstallationRepositories lists repos accessible to an installation.
func (s *server) handleGitHubInstallationRepositories(w http.ResponseWriter, r *http.Request, idRaw string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	installationID, err := strconv.ParseInt(strings.TrimSpace(idRaw), 10, 64)
	if err != nil || installationID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "invalid_installation_id"})
		return
	}
	cli, err := s.loadAppClient(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "app_client_load_failed"})
		return
	}
	if cli == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "error": "app_not_configured"})
		return
	}
	repos, err := cli.ListInstallationRepositories(r.Context(), installationID)
	if err != nil {
		s.requestLog(r).Error("list installation repositories failed", "installation_id", installationID, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "repositories_list_failed"})
		return
	}
	out := make([]apiGitHubRepo, 0, len(repos))
	for _, repo := range repos {
		out = append(out, apiGitHubRepo{
			ID:            repo.ID,
			Name:          repo.Name,
			FullName:      repo.FullName,
			Private:       repo.Private,
			DefaultBranch: repo.DefaultBranch,
			HTMLURL:       repo.HTMLURL,
			CloneURL:      repo.CloneURL,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"installation_id": installationID, "repositories": out})
}

// syncInstallationsFromAPI fetches installations from the GitHub App API and
// upserts them; returns the number upserted.
func (s *server) syncInstallationsFromAPI(ctx context.Context) (int, error) {
	cli, err := s.loadAppClient(ctx)
	if err != nil {
		return 0, err
	}
	if cli == nil {
		return 0, repository.ErrGitHubAppNotConfigured
	}
	items, err := cli.ListInstallations(ctx)
	if err != nil {
		return 0, err
	}
	seen := map[int64]struct{}{}
	for _, it := range items {
		seen[it.ID] = struct{}{}
		if err := s.store.UpsertGitHubInstallation(ctx, repository.UpsertGitHubInstallationInput{
			InstallationID: it.ID,
			AccountLogin:   it.Account.Login,
			AccountType:    it.Account.Type,
			TargetType:     it.TargetType,
			RepoSelection:  it.RepoSelection,
			Suspended:      strings.TrimSpace(it.SuspendedAt) != "",
		}); err != nil {
			return 0, err
		}
	}
	existing, err := s.store.ListGitHubInstallations(ctx)
	if err != nil {
		return len(items), nil
	}
	for _, row := range existing {
		if _, ok := seen[row.InstallationID]; !ok {
			_ = s.store.DeleteGitHubInstallation(ctx, row.InstallationID)
		}
	}
	return len(items), nil
}

// inferBaseURL picks a reasonable base URL from the incoming request when the
// UI does not supply one explicitly (e.g. for dev setups behind a reverse proxy).
func inferBaseURL(r *http.Request) string {
	scheme := "http"
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") || r.TLS != nil {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

// ensure githubapp import used in multiple spots
var _ = models.GitSourceGitHubApp
