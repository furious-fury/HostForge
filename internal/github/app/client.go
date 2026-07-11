package app

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultAPIBase is the public GitHub REST base URL.
const DefaultAPIBase = "https://api.github.com"

// DefaultUserAgent is sent with every request.
const DefaultUserAgent = "HostForge"

// Client talks to the GitHub REST API on behalf of a single GitHub App.
//
// The zero value is not usable; construct with New.
type Client struct {
	apiBase    string
	userAgent  string
	httpClient *http.Client
	appID      int64
	privateKey *rsa.PrivateKey
	now        func() time.Time

	mu             sync.Mutex
	installTokens  map[int64]*cachedToken
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

// Config configures a Client.
type Config struct {
	APIBase    string
	UserAgent  string
	HTTPClient *http.Client
	AppID      int64
	PrivateKey *rsa.PrivateKey
	Now        func() time.Time
}

// New builds a Client from Config. AppID and PrivateKey are required for any
// App-authenticated operation, but a Client can still call public endpoints
// (like ExchangeManifestCode) without them.
func New(cfg Config) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/")
	if base == "" {
		base = DefaultAPIBase
	}
	ua := strings.TrimSpace(cfg.UserAgent)
	if ua == "" {
		ua = DefaultUserAgent
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	n := cfg.Now
	if n == nil {
		n = time.Now
	}
	return &Client{
		apiBase:       base,
		userAgent:     ua,
		httpClient:    hc,
		appID:         cfg.AppID,
		privateKey:    cfg.PrivateKey,
		now:           n,
		installTokens: map[int64]*cachedToken{},
	}, nil
}

// AppID returns the configured App id (0 if unset).
func (c *Client) AppID() int64 { return c.appID }

// appJWT returns a fresh JWT signed by the App's private key.
func (c *Client) appJWT() (string, error) {
	if c == nil || c.privateKey == nil || c.appID <= 0 {
		return "", errors.New("app client not configured with app id + private key")
	}
	return MintAppJWT(c.appID, c.privateKey, c.now())
}

// doJSON performs an HTTP request with the given auth mode and decodes JSON response.
// authMode: "jwt" | "token" | "none".
func (c *Client) doJSON(ctx context.Context, method, path, auth, token string, body any, out any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(buf)
	}
	u := c.apiBase + path
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	switch auth {
	case "jwt":
		req.Header.Set("Authorization", "Bearer "+token)
	case "token":
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return resp, fmt.Errorf("github %s %s: %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(buf)))
	}
	if out != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp, nil
}

// ManifestCredentials is returned by POST /app-manifests/{code}/conversions.
type ManifestCredentials struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	HTMLURL       string `json:"html_url"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	PEM           string `json:"pem"`
	WebhookSecret string `json:"webhook_secret"`
}

// ExchangeManifestCode exchanges the one-time manifest code from GitHub for App credentials.
// This endpoint is unauthenticated (the code is short-lived and single-use).
func ExchangeManifestCode(ctx context.Context, apiBase, code string, hc *http.Client) (*ManifestCredentials, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("empty manifest code")
	}
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if base == "" {
		base = DefaultAPIBase
	}
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	u := base + "/app-manifests/" + url.PathEscape(code) + "/conversions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", DefaultUserAgent)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("manifest exchange: %d: %s", resp.StatusCode, strings.TrimSpace(string(buf)))
	}
	var out ManifestCredentials
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if out.ID <= 0 || strings.TrimSpace(out.PEM) == "" {
		return nil, errors.New("manifest response missing id or pem")
	}
	return &out, nil
}

// Installation is a minimal subset of the GitHub installation object.
type Installation struct {
	ID            int64  `json:"id"`
	TargetType    string `json:"target_type"`
	RepoSelection string `json:"repository_selection"`
	SuspendedAt   string `json:"suspended_at"`
	Account       struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
}

// ListInstallations returns every installation of the App (paginated; follows Link: next).
func (c *Client) ListInstallations(ctx context.Context) ([]Installation, error) {
	jwt, err := c.appJWT()
	if err != nil {
		return nil, err
	}
	var all []Installation
	path := "/app/installations?per_page=100"
	for path != "" {
		var page []Installation
		resp, err := c.doJSON(ctx, http.MethodGet, path, "jwt", jwt, nil, &page)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		path = parseNextLink(resp.Header.Get("Link"), c.apiBase)
	}
	return all, nil
}

// InstallationToken is the short-lived token for one installation.
type InstallationToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// MintInstallationToken returns a cached access token for the given installation,
// minting a new one if the cache is empty or expiring within 60s.
func (c *Client) MintInstallationToken(ctx context.Context, installationID int64) (InstallationToken, error) {
	if installationID <= 0 {
		return InstallationToken{}, errors.New("invalid installation id")
	}
	c.mu.Lock()
	if t, ok := c.installTokens[installationID]; ok {
		if c.now().Add(60 * time.Second).Before(t.expiresAt) {
			tok := InstallationToken{Token: t.token, ExpiresAt: t.expiresAt}
			c.mu.Unlock()
			return tok, nil
		}
	}
	c.mu.Unlock()

	jwt, err := c.appJWT()
	if err != nil {
		return InstallationToken{}, err
	}
	var out InstallationToken
	_, err = c.doJSON(
		ctx,
		http.MethodPost,
		"/app/installations/"+strconv.FormatInt(installationID, 10)+"/access_tokens",
		"jwt",
		jwt,
		nil,
		&out,
	)
	if err != nil {
		return InstallationToken{}, err
	}
	if strings.TrimSpace(out.Token) == "" {
		return InstallationToken{}, errors.New("empty installation token")
	}
	c.mu.Lock()
	c.installTokens[installationID] = &cachedToken{token: out.Token, expiresAt: out.ExpiresAt}
	c.mu.Unlock()
	return out, nil
}

// InvalidateInstallationToken drops any cached token for installationID; the next
// call to MintInstallationToken will fetch a fresh one.
func (c *Client) InvalidateInstallationToken(installationID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.installTokens, installationID)
}

// Repository is a minimal subset of a GitHub repo object.
type Repository struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

type repoListResponse struct {
	TotalCount   int          `json:"total_count"`
	Repositories []Repository `json:"repositories"`
}

// ListInstallationRepositories lists all repos accessible to an installation (paginated).
func (c *Client) ListInstallationRepositories(ctx context.Context, installationID int64) ([]Repository, error) {
	tok, err := c.MintInstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	var all []Repository
	path := "/installation/repositories?per_page=100"
	for path != "" {
		var page repoListResponse
		resp, err := c.doJSON(ctx, http.MethodGet, path, "token", tok.Token, nil, &page)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Repositories...)
		path = parseNextLink(resp.Header.Get("Link"), c.apiBase)
	}
	return all, nil
}

// ListRepositoryBranches returns all branch names for a repo accessible to the installation.
func (c *Client) ListRepositoryBranches(ctx context.Context, installationID int64, owner, repo string) ([]string, error) {
	tok, err := c.MintInstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	var names []string
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/branches?per_page=100"
	for path != "" {
		var page []struct {
			Name string `json:"name"`
		}
		resp, err := c.doJSON(ctx, http.MethodGet, path, "token", tok.Token, nil, &page)
		if err != nil {
			return nil, err
		}
		for _, b := range page {
			if n := strings.TrimSpace(b.Name); n != "" {
				names = append(names, n)
			}
		}
		path = parseNextLink(resp.Header.Get("Link"), c.apiBase)
	}
	return names, nil
}

// parseNextLink extracts the next-page path (starting with "/") from a Link header.
// Returns "" if absent. The apiBase is used to strip the scheme+host when present.
func parseNextLink(link, apiBase string) string {
	if strings.TrimSpace(link) == "" {
		return ""
	}
	for _, part := range strings.Split(link, ",") {
		part = strings.TrimSpace(part)
		segs := strings.Split(part, ";")
		if len(segs) < 2 {
			continue
		}
		u := strings.TrimSpace(segs[0])
		u = strings.TrimPrefix(u, "<")
		u = strings.TrimSuffix(u, ">")
		rel := ""
		for _, s := range segs[1:] {
			s = strings.TrimSpace(s)
			if strings.HasPrefix(s, "rel=") {
				rel = strings.Trim(strings.TrimPrefix(s, "rel="), `"`)
			}
		}
		if rel != "next" {
			continue
		}
		if strings.HasPrefix(u, apiBase) {
			return strings.TrimPrefix(u, apiBase)
		}
		if p, err := url.Parse(u); err == nil && p.Path != "" {
			q := p.RawQuery
			if q != "" {
				return p.Path + "?" + q
			}
			return p.Path
		}
	}
	return ""
}
