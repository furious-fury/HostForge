// Package version holds the release identifier embedded from the VERSION file.
// Bump [VERSION] in this directory. Commit and BuildTime are set at release
// build time via -ldflags (see .github/workflows/release.yml); a local
// `go build` leaves both empty.
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var raw string

// Commit is the git revision at build time (optional; set via -ldflags).
var Commit string

// BuildTime is an RFC3339 or human build timestamp (optional; set via -ldflags).
var BuildTime string

// String returns the single-line release version (e.g. "0.8.0").
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

// Display returns a v-prefixed release label for logs and UI (e.g. "v0.8.0").
func Display() string {
	return "v" + String()
}
