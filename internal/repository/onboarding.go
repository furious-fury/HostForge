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
	err := s.db.QueryRowContext(ctx, `
		SELECT CASE WHEN github_app_complete=1 OR EXISTS(SELECT 1 FROM github_app WHERE id=1) THEN 1 ELSE 0 END,
		       platform_domain,permanent_ingress_complete,bootstrap_complete,completed_at,updated_at
		FROM onboarding_state WHERE id=1`).Scan(&github, &out.PlatformDomain, &ingress, &complete, &completedAt, &updatedAt)
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
	res, err := s.db.ExecContext(ctx, `
		UPDATE onboarding_state
		SET github_app_complete=1,platform_domain=?,permanent_ingress_complete=1,bootstrap_complete=1,completed_at=?,updated_at=?
		WHERE id=1 AND (github_app_complete=1 OR EXISTS(SELECT 1 FROM github_app WHERE id=1))`,
		d, now, now)
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
	// Acquire SQLite's write reservation before inspecting gateway state. Every
	// gateway mutation also writes SQLite, so none can commit between this guard
	// and the platform-domain update.
	lockResult, err := tx.ExecContext(ctx, `UPDATE onboarding_state SET updated_at=updated_at WHERE id=1 AND platform_domain=?`, current)
	if err != nil {
		return fmt.Errorf("lock platform domain: %w", err)
	}
	locked, err := lockResult.RowsAffected()
	if err != nil {
		return err
	}
	if locked != 1 {
		return ErrPlatformDomainChanged
	}
	if err := checkDatabaseGatewayDomainChangeAllowed(ctx, tx); err != nil {
		return err
	}
	var mismatched int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM domains WHERE kind='platform' AND domain_name NOT LIKE ?`, "%."+current).Scan(&mismatched); err != nil {
		return err
	}
	if mismatched > 0 {
		return fmt.Errorf("managed platform domains do not match current suffix")
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		UPDATE domains
		SET domain_name=substr(domain_name,1,length(domain_name)-length(?)) || ?,
		    ssl_status='PENDING',last_cert_message='',cert_checked_at='',updated_at=?
		WHERE kind='platform'`,
		current, next, stamp); err != nil {
		return fmt.Errorf("move managed platform domains: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE database_gateway_endpoints
		SET hostname=substr(hostname,1,length(hostname)-length(?)) || ?,updated_at=?
		WHERE hostname=? OR hostname LIKE ?`, current, next, stamp, current, "%."+current); err != nil {
		return fmt.Errorf("move absent database gateway endpoints: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE onboarding_state SET platform_domain=?,updated_at=? WHERE id=1 AND platform_domain=?`, next, stamp, current)
	if err != nil {
		return fmt.Errorf("update platform domain: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrPlatformDomainChanged
	}
	return tx.Commit()
}
