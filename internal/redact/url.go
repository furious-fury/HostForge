// Package redact provides safe string forms for logs and API surfaces.
package redact

import (
	"net/url"
	"strings"
)

// RepoURLForLog returns a URL string with userinfo removed, suitable for slog fields.
func RepoURLForLog(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "[invalid_url]"
	}
	u.User = nil
	s := u.String()
	if s == "" {
		return "[invalid_url]"
	}
	return s
}

// HTTPURLForLog returns a URL string with userinfo removed; use for admin endpoints in logs.
func HTTPURLForLog(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "[invalid_url]"
	}
	u.User = nil
	s := u.String()
	if s == "" {
		return "[invalid_url]"
	}
	return s
}
