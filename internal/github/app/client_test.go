package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newClientAgainst(t *testing.T, ts *httptest.Server) *Client {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{
		APIBase:    ts.URL,
		AppID:      42,
		PrivateKey: key,
		HTTPClient: ts.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestExchangeManifestCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		if r.URL.Path != "/app-manifests/abc123/conversions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ManifestCredentials{
			ID:            999,
			Slug:          "hostforge-dev",
			HTMLURL:       "https://github.com/apps/hostforge-dev",
			ClientID:      "client-id",
			ClientSecret:  "client-secret",
			PEM:           "-----BEGIN RSA PRIVATE KEY-----\nXYZ\n-----END RSA PRIVATE KEY-----\n",
			WebhookSecret: "whsec",
		})
	}))
	defer ts.Close()

	ctx := context.Background()
	got, err := ExchangeManifestCode(ctx, ts.URL, "abc123", ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 999 || got.Slug != "hostforge-dev" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestExchangeManifestCode_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Bad code"}`, http.StatusUnprocessableEntity)
	}))
	defer ts.Close()
	ctx := context.Background()
	if _, err := ExchangeManifestCode(ctx, ts.URL, "xyz", ts.Client()); err == nil {
		t.Fatal("want error")
	}
}

func TestListInstallations_Pagination(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Errorf("missing bearer auth: %s", got)
		}
		if n == 1 {
			w.Header().Set("Link", fmt.Sprintf(`<%s/app/installations?per_page=100&page=2>; rel="next"`, strings.TrimRight("http://"+r.Host, "/")))
			_ = json.NewEncoder(w).Encode([]Installation{{ID: 1}, {ID: 2}})
			return
		}
		_ = json.NewEncoder(w).Encode([]Installation{{ID: 3}})
	}))
	defer ts.Close()
	c := newClientAgainst(t, ts)
	got, err := c.ListInstallations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 requests, got %d", calls)
	}
}

func TestMintInstallationToken_CacheAndInvalidate(t *testing.T) {
	var mint int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mint, 1)
		_ = json.NewEncoder(w).Encode(InstallationToken{
			Token:     fmt.Sprintf("ghs_token_%d", mint),
			ExpiresAt: time.Now().Add(time.Hour),
		})
	}))
	defer ts.Close()
	c := newClientAgainst(t, ts)

	t1, err := c.MintInstallationToken(context.Background(), 123)
	if err != nil {
		t.Fatal(err)
	}
	t2, err := c.MintInstallationToken(context.Background(), 123)
	if err != nil {
		t.Fatal(err)
	}
	if t1.Token != t2.Token {
		t.Fatalf("cache miss: %q vs %q", t1.Token, t2.Token)
	}
	if atomic.LoadInt32(&mint) != 1 {
		t.Fatalf("expected 1 mint, got %d", mint)
	}
	c.InvalidateInstallationToken(123)
	t3, err := c.MintInstallationToken(context.Background(), 123)
	if err != nil {
		t.Fatal(err)
	}
	if t3.Token == t1.Token {
		t.Fatal("expected new token after invalidate")
	}
}

func TestMintInstallationToken_ExpiryRefresh(t *testing.T) {
	var mint int32
	var now atomic.Value
	now.Store(time.Unix(1_700_000_000, 0).UTC())
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mint, 1)
		_ = json.NewEncoder(w).Encode(InstallationToken{
			Token:     fmt.Sprintf("tok%d", mint),
			ExpiresAt: now.Load().(time.Time).Add(time.Minute),
		})
	}))
	defer ts.Close()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := New(Config{
		APIBase:    ts.URL,
		AppID:      42,
		PrivateKey: key,
		HTTPClient: ts.Client(),
		Now:        func() time.Time { return now.Load().(time.Time) },
	})

	if _, err := c.MintInstallationToken(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	now.Store(now.Load().(time.Time).Add(90 * time.Second))
	if _, err := c.MintInstallationToken(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&mint) != 2 {
		t.Fatalf("expected refresh mint, got %d", mint)
	}
}

func TestListInstallationRepositories(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/installations/9/access_tokens":
			_ = json.NewEncoder(w).Encode(InstallationToken{Token: "t", ExpiresAt: time.Now().Add(time.Hour)})
		case "/installation/repositories":
			_ = json.NewEncoder(w).Encode(repoListResponse{
				TotalCount: 2,
				Repositories: []Repository{
					{ID: 1, Name: "api", FullName: "acme/api", DefaultBranch: "main"},
					{ID: 2, Name: "web", FullName: "acme/web", DefaultBranch: "develop"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	c := newClientAgainst(t, ts)
	repos, err := c.ListInstallationRepositories(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 || repos[0].FullName != "acme/api" {
		t.Fatalf("unexpected: %+v", repos)
	}
}

func TestListRepositoryBranches(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/app/installations/3/access_tokens":
			_ = json.NewEncoder(w).Encode(InstallationToken{Token: "t", ExpiresAt: time.Now().Add(time.Hour)})
		case r.URL.Path == "/repos/acme/api/branches":
			_ = json.NewEncoder(w).Encode([]struct {
				Name string `json:"name"`
			}{{Name: "main"}, {Name: "develop"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	c := newClientAgainst(t, ts)
	names, err := c.ListRepositoryBranches(context.Background(), 3, "acme", "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "main" {
		t.Fatalf("unexpected: %v", names)
	}
}

func TestParseNextLink(t *testing.T) {
	base := "https://api.github.com"
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{`<https://api.github.com/foo?page=2>; rel="next", <...>; rel="last"`, "/foo?page=2"},
		{`<https://api.github.com/foo>; rel="prev"`, ""},
		{`<https://other.example.com/bar?x=1>; rel="next"`, "/bar?x=1"},
	}
	for _, c := range cases {
		if got := parseNextLink(c.in, base); got != c.want {
			t.Errorf("parseNextLink(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
