// Package platformdomain assigns deterministic platform hostnames to projects.
package platformdomain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/hostforge/hostforge/internal/dnsops"
	"github.com/hostforge/hostforge/internal/models"
	"github.com/hostforge/hostforge/internal/repository"
)

// NormalizeBase validates the one-time wildcard DNS base, for example
// "apps.example.com". An empty value disables platform hostname assignment.
func NormalizeBase(raw string) (string, error) {
	base := strings.Trim(strings.ToLower(strings.TrimSpace(raw)), ".")
	if base == "" {
		return "", nil
	}
	if err := dnsops.ValidateDomainName(base); err != nil {
		return "", fmt.Errorf("platform domain base: %w", err)
	}
	return base, nil
}

// Ensure creates one platform hostname for project when a base is configured.
// Existing project domains under the configured base are preserved.
func Ensure(ctx context.Context, store *repository.Store, base string, project models.Project) (models.Domain, bool, error) {
	base, err := NormalizeBase(base)
	if err != nil || base == "" {
		return models.Domain{}, false, err
	}
	domains, err := store.ListDomainsByProject(ctx, project.ID)
	if err != nil {
		return models.Domain{}, false, err
	}
	for _, d := range domains {
		if isUnderBase(d.DomainName, base) {
			return d, false, nil
		}
	}
	name := Hostname(base, project.Name, "")
	d, err := store.CreateDomain(ctx, project.ID, name)
	if err == nil {
		return d, true, nil
	}
	if !errors.Is(err, repository.ErrDuplicateDomain) {
		return models.Domain{}, false, err
	}
	d, err = store.CreateDomain(ctx, project.ID, Hostname(base, project.Name, project.ID))
	if err != nil {
		return models.Domain{}, false, err
	}
	return d, true, nil
}

// Hostname creates a valid project hostname. A non-empty suffix is used only
// to resolve a project-name collision without compromising readability.
func Hostname(base, projectName, suffix string) string {
	label := slug(projectName)
	suffix = strings.TrimSpace(suffix)
	if suffix != "" {
		suffix = slug(suffix)
		label = trimLabel(label, 63-len(suffix)-1) + "-" + suffix
	}
	return label + "." + base
}

func isUnderBase(host, base string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.HasSuffix(host, "."+base) && strings.TrimSuffix(host, "."+base) != ""
}

func slug(raw string) string {
	var b strings.Builder
	previousDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			previousDash = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || !previousDash {
			b.WriteByte('-')
			previousDash = true
		}
	}
	return trimLabel(b.String(), 63)
}

func trimLabel(raw string, max int) string {
	if max < 1 {
		return "app"
	}
	s := strings.Trim(raw, "-")
	if s == "" {
		s = "app"
	}
	if len(s) > max {
		s = strings.TrimRight(s[:max], "-")
	}
	if s == "" {
		return "app"
	}
	return s
}
