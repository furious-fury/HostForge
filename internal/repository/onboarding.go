package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/models"
)

// GetOnboardingState returns the singleton onboarding state.
func (s *Store) GetOnboardingState(ctx context.Context) (models.OnboardingState, error) {
	var out models.OnboardingState
	var github, ingress, complete int
	var completedAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT github_app_complete, platform_domain, permanent_ingress_complete, bootstrap_complete, completed_at, updated_at FROM onboarding_state WHERE id=1`).Scan(&github, &out.PlatformDomain, &ingress, &complete, &completedAt, &updatedAt)
	if err != nil {
		return out, fmt.Errorf("get onboarding state: %w", err)
	}
	out.GitHubAppComplete, out.PermanentIngressComplete, out.BootstrapComplete = github != 0, ingress != 0, complete != 0
	out.CompletedAt, out.UpdatedAt = parseTime(completedAt), parseTime(updatedAt)
	return out, nil
}

// MarkGitHubAppComplete records a successfully persisted GitHub App.
func (s *Store) MarkGitHubAppComplete(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE onboarding_state SET github_app_complete=1, updated_at=? WHERE id=1`, time.Now().UTC().Format(time.RFC3339))
	return err
}

// CompleteOnboarding records the permanent ingress only after the caller has validated and reloaded it.
func (s *Store) CompleteOnboarding(ctx context.Context, domain string) error {
	d := strings.ToLower(strings.TrimSpace(domain))
	if d == "" {
		return fmt.Errorf("platform domain is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `UPDATE onboarding_state SET platform_domain=?, permanent_ingress_complete=1, bootstrap_complete=1, completed_at=?, updated_at=? WHERE id=1 AND github_app_complete=1`, d, now, now)
	if err != nil {
		return fmt.Errorf("complete onboarding: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("github app must be completed first")
	}
	return nil
}

// UpdatePlatformDomain changes the control-plane hostname and preserves every
// managed share URL label while moving it beneath the new platform domain.
func (s *Store) UpdatePlatformDomain(ctx context.Context, currentDomain, nextDomain string) error {
	current := strings.ToLower(strings.TrimSpace(currentDomain))
	next := strings.ToLower(strings.TrimSpace(nextDomain))
	if current == "" || next == "" {
		return fmt.Errorf("current and next platform domains are required")
	}
	if current == next {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var mismatched int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM domains WHERE kind='platform' AND domain_name NOT LIKE ?`, "%."+current).Scan(&mismatched); err != nil {
		return err
	}
	if mismatched > 0 {
		return fmt.Errorf("managed platform domains do not match current suffix")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE domains
		SET domain_name=substr(domain_name,1,length(domain_name)-length(?)) || ?,
		    ssl_status='PENDING',last_cert_message='',cert_checked_at='',updated_at=?
		WHERE kind='platform'`,
		current, next, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("move managed platform domains: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE onboarding_state SET platform_domain=?,updated_at=? WHERE id=1 AND platform_domain=?`, next, time.Now().UTC().Format(time.RFC3339), current)
	if err != nil {
		return fmt.Errorf("update platform domain: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return fmt.Errorf("platform domain changed concurrently")
	}
	return tx.Commit()
}
