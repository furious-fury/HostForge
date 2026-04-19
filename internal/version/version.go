// Package version holds the release identifier embedded from the VERSION file.
// Bump [VERSION] in this directory; the web UI reads the same file at build time (see web/vite.config.ts).
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var raw string

// String returns the single-line release version (e.g. "0.7.0").
func String() string {
	s := strings.TrimSpace(raw)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if s == "" {
		return "0.0.0-dev"
	}
	return strings.TrimPrefix(s, "v")
}

// Display returns a v-prefixed release label for logs and UI (e.g. "v0.7.0").
func Display() string {
	return "v" + String()
}
