package caddy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ErrValidationFailed marks a Sync failure as a rejected config -- content
// Caddy itself refused (a malformed hostname, a conflicting site block) --
// as opposed to a reload failure, which means the config was valid but
// applying it failed for an unrelated reason (Caddy not running, a process
// error). Callers use errors.Is to tell the two apart: only the former
// implicates a specific domain (ADR-0002 §19.2).
var ErrValidationFailed = errors.New("caddy: config rejected by validate")

// Route maps a domain to a local upstream host port.
type Route struct {
	Domain   string
	HostPort int
}

// SyncOptions controls generated config output and caddy apply behavior.
type SyncOptions struct {
	CaddyBin      string
	GeneratedPath string
	RootConfig    string
	Routes        []Route
	// CertificateDomains are HTTPS certificate-only sites. They never proxy
	// database protocol traffic; Caddy is used solely as the ACME issuer.
	CertificateDomains []string
}

// SyncResult describes the generated output path and whether caddy was reloaded.
type SyncResult struct {
	GeneratedPath string
	Applied       bool
}

// Sync renders routes, writes generated config atomically, validates, then reloads caddy.
func Sync(ctx context.Context, opts SyncOptions) (SyncResult, error) {
	if strings.TrimSpace(opts.GeneratedPath) == "" {
		return SyncResult{}, fmt.Errorf("generated path is required")
	}
	if strings.TrimSpace(opts.RootConfig) == "" {
		return SyncResult{}, fmt.Errorf("root config path is required")
	}
	bin := strings.TrimSpace(opts.CaddyBin)
	if bin == "" {
		bin = "caddy"
	}
	content := RenderConfigWithCertificateDomains(opts.Routes, opts.CertificateDomains)
	if err := writeAtomic(opts.GeneratedPath, []byte(content)); err != nil {
		return SyncResult{}, err
	}
	if err := ValidateRoot(ctx, bin, opts.RootConfig); err != nil {
		return SyncResult{GeneratedPath: opts.GeneratedPath}, fmt.Errorf("%w: %w", ErrValidationFailed, err)
	}
	if err := runCaddy(ctx, bin, "reload", "--config", opts.RootConfig); err != nil {
		// `caddy reload` talks to the admin API (default :2019). When no daemon is running yet,
		// reload fails even though validate passed and the snippet is on disk for `caddy run` / systemctl.
		if isCaddyAdminUnreachable(err) {
			return SyncResult{GeneratedPath: opts.GeneratedPath, Applied: false}, nil
		}
		return SyncResult{GeneratedPath: opts.GeneratedPath}, fmt.Errorf("caddy reload: %w", err)
	}
	return SyncResult{GeneratedPath: opts.GeneratedPath, Applied: true}, nil
}

// ReplaceManagedConfig atomically updates a HostForge-managed Caddy snippet,
// validates the importing root Caddyfile, and reloads Caddy. If validation or
// reload fails, the previous snippet is restored before returning.
func ReplaceManagedConfig(ctx context.Context, caddyBin, managedPath, rootConfig, content string) error {
	target := strings.TrimSpace(managedPath)
	if target == "" {
		return fmt.Errorf("managed config path is required")
	}
	root := strings.TrimSpace(rootConfig)
	if root == "" {
		return fmt.Errorf("root config path is required")
	}
	previous, readErr := os.ReadFile(target)
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("read current managed config: %w", readErr)
	}
	if err := writeAtomic(target, []byte(content)); err != nil {
		return err
	}
	bin := strings.TrimSpace(caddyBin)
	if bin == "" {
		bin = "caddy"
	}
	if err := ValidateRoot(ctx, bin, root); err != nil {
		if readErr == nil {
			_ = writeAtomic(target, previous)
		} else {
			_ = os.Remove(target)
		}
		return fmt.Errorf("validate managed caddy config: %w", err)
	}
	if err := runCaddy(ctx, bin, "reload", "--config", root); err != nil {
		if readErr == nil {
			_ = writeAtomic(target, previous)
			_ = runCaddy(ctx, bin, "reload", "--config", root)
		} else {
			_ = os.Remove(target)
			_ = runCaddy(ctx, bin, "reload", "--config", root)
		}
		return fmt.Errorf("reload managed caddy config: %w", err)
	}
	return nil
}
func isCaddyAdminUnreachable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connection refused") && strings.Contains(s, "2019")
}

// ValidateSiteBlock validates one route as a complete, standalone Caddyfile --
// no root import, no other domains. It exists so a typo is rejected
// synchronously, by the operator who made it, with a useful message,
// instead of surfacing later as a fleet-wide reconcile failure that
// implicates whichever domain happens to sort last (ADR-0002 §19.3 item 1).
// The temp file is removed on return; nothing it writes is durable.
//
// Built directly rather than through RenderConfig: that renderer drops any
// route with hostPort <= 0, which describes most domains at add time -- the
// target service usually has no successful deployment yet. `caddy validate`
// never dials the upstream, only parses the Caddyfile, so an unresolved port
// stands in as a syntactic placeholder without weakening the check.
func ValidateSiteBlock(ctx context.Context, caddyBin, domain string, hostPort int) error {
	bin := strings.TrimSpace(caddyBin)
	if bin == "" {
		bin = "caddy"
	}
	domain = strings.TrimSpace(domain)
	port := hostPort
	if port <= 0 {
		port = 1
	}
	content := fmt.Sprintf("%s {\n    reverse_proxy 127.0.0.1:%d\n}\n", domain, port)
	tmp, err := os.CreateTemp("", ".hostforge-caddy-validate-*.caddyfile")
	if err != nil {
		return fmt.Errorf("create temp validation file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp validation file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp validation file: %w", err)
	}
	if err := runCaddy(ctx, bin, "validate", "--config", tmpPath, "--adapter", "caddyfile"); err != nil {
		return fmt.Errorf("%w: %w", ErrValidationFailed, err)
	}
	return nil
}

// ValidateRoot runs `caddy validate` against an existing root Caddyfile (no reload, no snippet write).
func ValidateRoot(ctx context.Context, caddyBin, rootConfig string) error {
	root := strings.TrimSpace(rootConfig)
	if root == "" {
		return fmt.Errorf("root config path is required")
	}
	bin := strings.TrimSpace(caddyBin)
	if bin == "" {
		bin = "caddy"
	}
	return runCaddy(ctx, bin, "validate", "--config", root)
}

// ValidateRootCapture runs `caddy validate` and returns separate stdout/stderr streams.
func ValidateRootCapture(ctx context.Context, caddyBin, rootConfig string) (stdout, stderr string, err error) {
	root := strings.TrimSpace(rootConfig)
	if root == "" {
		return "", "", fmt.Errorf("root config path is required")
	}
	bin := strings.TrimSpace(caddyBin)
	if bin == "" {
		bin = "caddy"
	}
	cmd := exec.CommandContext(ctx, bin, "validate", "--config", root)
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	err = cmd.Run()
	return outb.String(), errb.String(), err
}

// RenderConfig converts routes into caddyfile server blocks.
func RenderConfig(routes []Route) string {
	return RenderConfigWithCertificateDomains(routes, nil)
}

// RenderConfigWithCertificateDomains includes isolated HTTPS sites whose only
// purpose is ACME certificate issuance for non-HTTP HostForge gateways.
func RenderConfigWithCertificateDomains(routes []Route, certificateDomains []string) string {
	filtered := make([]Route, 0, len(routes))
	for _, route := range routes {
		if route.HostPort <= 0 || strings.TrimSpace(route.Domain) == "" {
			continue
		}
		route.Domain = strings.TrimSpace(route.Domain)
		filtered = append(filtered, route)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Domain < filtered[j].Domain
	})

	var b strings.Builder
	b.WriteString("# generated by hostforge; do not edit manually\n\n")
	for _, route := range filtered {
		fmt.Fprintf(&b, "%s {\n", route.Domain)
		fmt.Fprintf(&b, "    reverse_proxy 127.0.0.1:%d\n", route.HostPort)
		b.WriteString("}\n\n")
	}
	seen := map[string]struct{}{}
	for _, route := range filtered {
		seen[strings.ToLower(route.Domain)] = struct{}{}
	}
	sort.Strings(certificateDomains)
	for _, domain := range certificateDomains {
		domain = strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
		if domain == "" {
			continue
		}
		if _, duplicate := seen[domain]; duplicate {
			continue
		}
		seen[domain] = struct{}{}
		fmt.Fprintf(&b, "https://%s {\n", domain)
		b.WriteString("    respond /hostforge-certificate-probe 204\n")
		b.WriteString("    respond 404\n")
		b.WriteString("}\n\n")
	}
	return b.String()
}

// RenderBootstrapConfig renders a standalone, HTTPS-only control-plane route.
// It deliberately emits no HTTP listener or redirect route.
//
// `tls internal` rather than a bare `tls`: Caddy 2.11 rejects the directive
// with no argument, which fails validation and leaves the control plane
// unreachable. `internal` is also the only honest option for a raw IP
// address, which no public CA will issue for -- this is Caddy's local CA and
// browsers warn until a real domain replaces it.
func RenderBootstrapConfig(publicIP string, httpsPort int) string {
	address := "https://" + publicIP
	if httpsPort != 443 {
		address += fmt.Sprintf(":%d", httpsPort)
	}
	return "{\n\tauto_https disable_redirects\n}\n\n# generated by hostforge bootstrap; HTTPS only, internal CA on a raw IP\n" + address + " {\n\ttls internal\n\treverse_proxy 127.0.0.1:8080 {\n\t\theader_up X-Forwarded-For {remote_host}\n\t}\n}\n"
}

// RenderPermanentControlPlaneConfig renders the post-bootstrap control-plane
// route stored in the root Caddyfile's managed control-plane import.
//
// No `tls` directive at all. A bare one is invalid in Caddy 2.11 and failed
// validation, which is what made completing onboarding impossible. The
// alternative -- `tls internal`, as the raw-IP bootstrap route uses -- would
// be worse than nothing here: this is a real hostname, so automatic HTTPS
// obtains a publicly trusted certificate for it, and forcing the internal CA
// would leave every visitor with a warning permanently. This matches how
// RenderConfigWithCertificateDomains already relies on automatic HTTPS.
func RenderPermanentControlPlaneConfig(domain string) string {
	return "# generated by hostforge onboarding\nhttps://" + strings.TrimSpace(domain) + " {\n\treverse_proxy 127.0.0.1:8080 {\n\t\theader_up X-Forwarded-For {remote_host}\n\t}\n}\n"
}
func writeAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir caddy dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".hostforge-caddy-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp caddy file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	// The generated file normally lives in a setgid caddy-owned directory so
	// the HostForge service can update it while the Caddy service imports it.
	// CreateTemp defaults to 0600; make the resulting file group-readable
	// before the atomic rename so Caddy can read the imported route snippet.
	if err := tmp.Chmod(0o640); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set generated caddy file permissions: %w", err)
	}

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp caddy file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp caddy file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace caddy config: %w", err)
	}
	return nil
}

func runCaddy(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(combined.String())
		if out == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}
