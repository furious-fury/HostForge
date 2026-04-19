// Package sysstatus reports coarse runtime health for the management UI.
package sysstatus

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hostforge/hostforge/internal/caddy"
	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/docker"
)

// BuildVersion is shown on the dashboard; bump with releases.
const BuildVersion = "v0.6.0 · phase 6"

// Row is one line in the System panel.
type Row struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Response is returned by GET /api/system/status.
type Response struct {
	Version string `json:"version"`
	Checks  []Row  `json:"checks"`
}

// Gather runs quick local checks (Docker ping, Caddy validate + listen, webhook route).
// Independent checks run concurrently so wall time is roughly max(check), not sum(check).
func Gather(ctx context.Context, cfg *config.Config) Response {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	checks := make([]Row, 3)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		cctx, c := context.WithTimeout(ctx, 5*time.Second)
		defer c()
		checks[0] = checkDocker(cctx)
	}()
	go func() {
		defer wg.Done()
		cctx, c := context.WithTimeout(ctx, 10*time.Second)
		defer c()
		checks[1] = checkCaddy(cctx, cfg)
	}()
	go func() {
		defer wg.Done()
		cctx, c := context.WithTimeout(ctx, 4*time.Second)
		defer c()
		checks[2] = checkWebhookRoute(cctx, cfg)
	}()
	wg.Wait()

	return Response{
		Version: BuildVersion,
		Checks:  checks,
	}
}

var (
	gatherCacheMu sync.Mutex
	gatherCache   struct {
		resp    Response
		expires time.Time
	}
	gatherCacheTTL = 5 * time.Second
)

// GatherCached returns a recent Gather snapshot when still fresh (TTL), otherwise runs Gather.
// Reduces repeated subprocess / Docker / HTTP probe cost when the UI polls or revisits quickly.
func GatherCached(ctx context.Context, cfg *config.Config) Response {
	now := time.Now()
	gatherCacheMu.Lock()
	if !gatherCache.expires.IsZero() && now.Before(gatherCache.expires) {
		r := gatherCache.resp
		gatherCacheMu.Unlock()
		return r
	}
	gatherCacheMu.Unlock()

	r := Gather(ctx, cfg)

	gatherCacheMu.Lock()
	gatherCache.resp = r
	gatherCache.expires = time.Now().Add(gatherCacheTTL)
	gatherCacheMu.Unlock()
	return r
}

func checkDocker(ctx context.Context) Row {
	cli, err := docker.NewClient(ctx)
	if err != nil {
		return Row{ID: "docker", Label: "Docker daemon", Status: "DOWN", Detail: truncate(err.Error(), 220)}
	}
	_ = cli.Close()
	return Row{ID: "docker", Label: "Docker daemon", Status: "RUNNING"}
}

func checkCaddy(ctx context.Context, cfg *config.Config) Row {
	root := strings.TrimSpace(cfg.CaddyRootConfig)
	if root == "" {
		return Row{
			ID:     "caddy",
			Label:  "Caddy (HTTPS)",
			Status: "SKIPPED",
			Detail: "Set HOSTFORGE_CADDY_ROOT_CONFIG so HostForge can run caddy validate against your root Caddyfile.",
		}
	}
	if err := caddy.ValidateRoot(ctx, cfg.CaddyBin, root); err != nil {
		return Row{ID: "caddy", Label: "Caddy (HTTPS)", Status: "ERROR", Detail: truncate(err.Error(), 280)}
	}
	if !tcpOpen(ctx, "127.0.0.1:443", 600*time.Millisecond) && !tcpOpen(ctx, "127.0.0.1:80", 600*time.Millisecond) {
		return Row{
			ID:     "caddy",
			Label:  "Caddy (HTTPS)",
			Status: "WARNING",
			Detail: "caddy validate passed, but nothing accepted TCP on localhost :80 or :443. Start caddy (e.g. systemctl start caddy) so browsers can connect.",
		}
	}
	return Row{ID: "caddy", Label: "Caddy (HTTPS)", Status: "READY"}
}

func checkWebhookRoute(ctx context.Context, cfg *config.Config) Row {
	base := loopbackHTTPBase(cfg.ListenAddr)
	path := cfg.WebhookBasePath
	if path == "" {
		path = "/hooks/github"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := strings.TrimSuffix(base, "/") + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Row{ID: "webhooks", Label: "Webhook route", Status: "ERROR", Detail: truncate(err.Error(), 200)}
	}
	client := &http.Client{Timeout: 4 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return Row{ID: "webhooks", Label: "Webhook route", Status: "DOWN", Detail: truncate(err.Error(), 220)}
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 512))
	// GET is not allowed on the GitHub webhook handler; 405 means the route is mounted.
	if res.StatusCode == http.StatusMethodNotAllowed {
		return Row{ID: "webhooks", Label: "Webhook route", Status: "READY"}
	}
	return Row{
		ID:     "webhooks",
		Label:  "Webhook route",
		Status: "WARNING",
		Detail: fmt.Sprintf("Expected HTTP 405 on GET %s; got %d", path, res.StatusCode),
	}
}

func tcpOpen(ctx context.Context, addr string, d time.Duration) bool {
	var dialer net.Dialer
	cctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	c, err := dialer.DialContext(cctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func loopbackHTTPBase(listen string) string {
	listen = strings.TrimSpace(listen)
	if listen == "" {
		return "http://127.0.0.1:8080"
	}
	if strings.HasPrefix(listen, ":") {
		return "http://127.0.0.1" + listen
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	if host == "" || host == "0.0.0.0" || host == "[::]" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Count(host, ":") >= 1 {
		return fmt.Sprintf("http://[%s]:%s", host, port)
	}
	return "http://" + net.JoinHostPort(host, port)
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
