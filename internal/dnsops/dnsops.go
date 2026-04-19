// Package dnsops builds operator-facing DNS record suggestions for custom hostnames.
package dnsops

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/config"
)

// SuggestedRecord is one row to enter in a DNS manager (semantics vary by provider UI).
type SuggestedRecord struct {
	Type     string `json:"type"`               // A or AAAA
	Name     string `json:"name"`               // @, www, or subdomain label
	Value    string `json:"value"`              // IPv4 or IPv6
	ZoneHint string `json:"zone_hint,omitempty"` // DNS zone apex (best-effort)
	Note     string `json:"note,omitempty"`
}

// Guidance is returned with domain APIs so operators can update DNS without guessing.
type Guidance struct {
	IPv4       string            `json:"ipv4,omitempty"`
	IPv6       string            `json:"ipv6,omitempty"`
	IPv4Source string            `json:"ipv4_source"` // override | detected | unknown
	IPv6Source string            `json:"ipv6_source"` // override | detected | unknown | omitted
	Records    []SuggestedRecord `json:"records"`
	Steps      []string            `json:"steps,omitempty"` // short numbered instructions per hostname
	Message    string            `json:"message,omitempty"`
}

var fqdnPattern = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])\.)+[a-z]{2,}$`)

// ValidateDomainName returns an error if host is not a plausible public DNS hostname.
func ValidateDomainName(host string) error {
	h := strings.TrimSpace(strings.ToLower(host))
	if h == "" {
		return fmt.Errorf("domain name is empty")
	}
	if len(h) > 253 {
		return fmt.Errorf("domain name too long")
	}
	if !fqdnPattern.MatchString(h) {
		return fmt.Errorf("domain name must be a valid hostname (e.g. app.example.com)")
	}
	if strings.Contains(h, "..") || strings.HasPrefix(h, ".") || strings.HasSuffix(h, ".") {
		return fmt.Errorf("domain name is invalid")
	}
	return nil
}

// ResolveExpectedIPv4 returns the IPv4 HostForge suggests for A records (override or HTTP detect).
func ResolveExpectedIPv4(ctx context.Context, cfg *config.Config) (ip string, source string, warn string) {
	timeout := time.Duration(cfg.DNSDetectTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 2500 * time.Millisecond
	}
	detectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if v := strings.TrimSpace(cfg.DNSServerIPv4); v != "" {
		if parsed := net.ParseIP(v); parsed == nil || parsed.To4() == nil {
			return "", "unknown", "HOSTFORGE_DNS_SERVER_IPV4 is set but not a valid IPv4 address; DNS A record values are omitted."
		} else {
			return parsed.To4().String(), "override", ""
		}
	}
	ip, err := detectPublicIPv4(detectCtx, cfg.DNSDetectURL)
	if err != nil || ip == "" {
		return "", "unknown", "Could not auto-detect public IPv4. Set HOSTFORGE_DNS_SERVER_IPV4 to your VPS address, or verify outbound HTTPS from this host."
	}
	return ip, "detected", ""
}

// CheckRegistrarARecord compares public DNS A/AAAA resolution for hostname to expectedIPv4.
// Returns status: ok | pending | unknown (no expected IP) | lookup_error, and any IPv4 addresses seen.
func CheckRegistrarARecord(ctx context.Context, hostname, expectedIPv4 string, lookupTimeout time.Duration) (status string, resolved []string) {
	want := strings.TrimSpace(expectedIPv4)
	if want == "" {
		return "unknown", nil
	}
	wantIP := net.ParseIP(want)
	if wantIP == nil || wantIP.To4() == nil {
		return "unknown", nil
	}
	want4 := wantIP.To4()
	if lookupTimeout <= 0 {
		lookupTimeout = 2500 * time.Millisecond
	}
	lctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(lctx, "ip4", strings.TrimSpace(hostname))
	if err != nil {
		return "lookup_error", nil
	}
	seen := make(map[string]struct{})
	var list []string
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			s := v4.String()
			if _, ok := seen[s]; !ok {
				seen[s] = struct{}{}
				list = append(list, s)
			}
			if v4.Equal(want4) {
				return "ok", list
			}
		}
	}
	return "pending", list
}

// BuildGuidance resolves server IPs from config (override or HTTP detect) and returns suggested DNS rows for each hostname.
func BuildGuidance(ctx context.Context, cfg *config.Config, hostnames []string) Guidance {
	ipv4, v4src, v4warn := ResolveExpectedIPv4(ctx, cfg)
	return BuildGuidanceWithIPv4(ctx, cfg, hostnames, ipv4, v4src, v4warn)
}

// BuildGuidanceWithIPv4 builds DNS guidance using a pre-resolved IPv4 (and optional warning from detection).
// Skips another outbound IPv4 detect; use after ResolveExpectedIPv4 or when IPv4 is known from config.
func BuildGuidanceWithIPv4(ctx context.Context, cfg *config.Config, hostnames []string, ipv4, v4src, v4warn string) Guidance {
	out := Guidance{
		IPv4Source: "unknown",
		IPv6Source: "omitted",
		Records:    []SuggestedRecord{},
	}
	v6timeout := time.Duration(cfg.DNSDetectTimeoutMS) * time.Millisecond
	if v6timeout <= 0 {
		v6timeout = 2500 * time.Millisecond
	}
	v6ctx, cancelV6 := context.WithTimeout(ctx, v6timeout)
	defer cancelV6()

	out.IPv4 = ipv4
	out.IPv4Source = v4src
	if v4warn != "" {
		out.Message = v4warn
	}

	if v := strings.TrimSpace(cfg.DNSServerIPv6); v != "" {
		ip := net.ParseIP(v)
		if ip == nil || ip.To4() != nil {
			if out.Message == "" {
				out.Message = "HOSTFORGE_DNS_SERVER_IPV6 is set but not a valid IPv6 address; AAAA suggestions are skipped."
			}
			out.IPv6Source = "unknown"
		} else {
			out.IPv6 = ip.String()
			out.IPv6Source = "override"
		}
	} else if u := strings.TrimSpace(cfg.DNSDetectIPv6URL); u != "" {
		ipStr, err := detectPublicIP(v6ctx, u)
		if err == nil && ipStr != "" {
			parsed := net.ParseIP(ipStr)
			if parsed != nil && parsed.To4() == nil {
				out.IPv6 = parsed.String()
				out.IPv6Source = "detected"
			}
		}
	}

	for _, raw := range hostnames {
		h := strings.TrimSpace(strings.ToLower(raw))
		if h == "" {
			continue
		}
		labels := strings.Split(h, ".")
		if len(labels) < 2 {
			continue
		}
		apex := strings.Join(labels[len(labels)-2:], ".")
		if len(labels) == 2 {
			// apex host e.g. mrfury.dev
			if out.IPv4 != "" {
				out.Records = append(out.Records, SuggestedRecord{
					Type:     "A",
					Name:     "@",
					Value:    out.IPv4,
					ZoneHint: apex,
					Note:     fmt.Sprintf("In your DNS zone for %s, create an A record: name @ (or blank), value %s", apex, out.IPv4),
				})
			}
			if out.IPv6 != "" {
				out.Records = append(out.Records, SuggestedRecord{
					Type:     "AAAA",
					Name:     "@",
					Value:    out.IPv6,
					ZoneHint: apex,
					Note:     fmt.Sprintf("Optional AAAA for %s at apex.", apex),
				})
			}
			if out.IPv4 != "" {
				out.Records = append(out.Records, SuggestedRecord{
					Type:     "A",
					Name:     "www",
					Value:    out.IPv4,
					ZoneHint: apex,
					Note:     fmt.Sprintf("Optional: A record for www.%s (same IP as apex) if you want www.", apex),
				})
			}
		} else {
			sub := labels[0]
			zone := strings.Join(labels[1:], ".")
			if out.IPv4 != "" {
				out.Records = append(out.Records, SuggestedRecord{
					Type:     "A",
					Name:     sub,
					Value:    out.IPv4,
					ZoneHint: zone,
					Note:     fmt.Sprintf("In DNS for zone %s, add A record host %s → %s", zone, sub, out.IPv4),
				})
			}
			if out.IPv6 != "" {
				out.Records = append(out.Records, SuggestedRecord{
					Type:     "AAAA",
					Name:     sub,
					Value:    out.IPv6,
					ZoneHint: zone,
					Note:     fmt.Sprintf("Optional AAAA for %s in zone %s.", h, zone),
				})
			}
		}
	}

	out.Steps = buildSteps(hostnames, out.IPv4, out.IPv6)
	return out
}

func buildSteps(hostnames []string, ipv4, ipv6 string) []string {
	if strings.TrimSpace(ipv4) == "" && strings.TrimSpace(ipv6) == "" {
		return nil
	}
	var steps []string
	for _, raw := range hostnames {
		h := strings.TrimSpace(strings.ToLower(raw))
		if h == "" {
			continue
		}
		labels := strings.Split(h, ".")
		if len(labels) < 2 {
			continue
		}
		if len(labels) == 2 {
			zone := h
			if ipv4 != "" {
				steps = append(steps, fmt.Sprintf("Apex %s: in your DNS zone %s, add an A record where Host/Name is @ (or blank), Points to/Value is %s.", h, zone, ipv4))
				steps = append(steps, fmt.Sprintf("Optional: same zone %s, add A where Host/Name is www, Points to/Value is %s, if you want www.%s.", zone, ipv4, h))
			}
			continue
		}
		sub := labels[0]
		zone := strings.Join(labels[1:], ".")
		if ipv4 != "" {
			steps = append(steps, fmt.Sprintf("Subdomain %s: in DNS zone %s (not HostForge), add A where Host/Name is only %s (not the full hostname), Points to/Value is %s.", h, zone, sub, ipv4))
		}
	}
	return steps
}

func detectPublicIPv4(ctx context.Context, urlStr string) (string, error) {
	return detectPublicIP(ctx, urlStr)
}

func detectPublicIP(ctx context.Context, urlStr string) (string, error) {
	u := strings.TrimSpace(urlStr)
	if u == "" {
		return "", fmt.Errorf("no detect URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "", fmt.Errorf("empty body")
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return "", fmt.Errorf("not an IP: %q", s)
	}
	return ip.String(), nil
}
