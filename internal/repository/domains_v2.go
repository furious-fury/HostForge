package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/models"
)

type ServiceDomain struct {
	ID               string `json:"id"`
	ApplicationID    string `json:"application_id"`
	EnvironmentID    string `json:"environment_id"`
	ServiceID        string `json:"service_id"`
	DomainName       string `json:"domain_name"`
	Kind             string `json:"kind"`
	SSLStatus        string `json:"ssl_status"`
	LastCertMessage  string `json:"last_cert_message,omitempty"`
	CertCheckedAtRaw string `json:"cert_checked_at,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func scanServiceDomain(scanner interface{ Scan(...any) error }) (ServiceDomain, error) {
	var item ServiceDomain
	err := scanner.Scan(&item.ID, &item.ApplicationID, &item.EnvironmentID, &item.ServiceID, &item.DomainName, &item.Kind, &item.SSLStatus, &item.LastCertMessage, &item.CertCheckedAtRaw, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

const serviceDomainColumns = `id,application_id,environment_id,service_id,domain_name,kind,ssl_status,last_cert_message,cert_checked_at,created_at,updated_at`

func (s *Store) CreateServiceDomain(ctx context.Context, applicationID, environmentID, serviceID, domainName string) (ServiceDomain, error) {
	applicationID = strings.TrimSpace(applicationID)
	environmentID = strings.TrimSpace(environmentID)
	serviceID = strings.TrimSpace(serviceID)
	domainName = strings.TrimSpace(domainName)
	var valid int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM service_environments se
		JOIN services svc ON svc.id=se.service_id
		JOIN environments env ON env.id=se.environment_id
		WHERE svc.application_id=? AND env.application_id=? AND se.environment_id=? AND se.service_id=?`,
		applicationID, applicationID, environmentID, serviceID).Scan(&valid); err != nil {
		return ServiceDomain{}, err
	}
	if valid == 0 {
		return ServiceDomain{}, ErrEnvironmentNotFound
	}
	now := time.Now().UTC().Format(time.RFC3339)
	id := newID()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO domains(id,application_id,environment_id,service_id,domain_name,kind,ssl_status,last_cert_message,cert_checked_at,created_at,updated_at)
		VALUES(?,?,?,?,?,'custom',?,?,?,?,?)`,
		id, applicationID, environmentID, serviceID, domainName, models.SSLStatusPending, "", "", now, now)
	if err != nil {
		if isUniqueConstraint(err) {
			return ServiceDomain{}, ErrDuplicateDomain
		}
		return ServiceDomain{}, fmt.Errorf("insert service domain: %w", err)
	}
	return s.GetServiceDomain(ctx, applicationID, environmentID, id)
}

func (s *Store) GetServiceDomain(ctx context.Context, applicationID, environmentID, id string) (ServiceDomain, error) {
	item, err := scanServiceDomain(s.db.QueryRowContext(ctx, `SELECT `+serviceDomainColumns+` FROM domains WHERE id=? AND application_id=? AND environment_id=?`, strings.TrimSpace(id), strings.TrimSpace(applicationID), strings.TrimSpace(environmentID)))
	if err == sql.ErrNoRows {
		return ServiceDomain{}, ErrDomainNotFound
	}
	return item, err
}

func (s *Store) ListServiceDomains(ctx context.Context, applicationID, environmentID, serviceID string) ([]ServiceDomain, error) {
	query := `SELECT ` + serviceDomainColumns + ` FROM domains WHERE application_id=? AND environment_id=?`
	args := []any{strings.TrimSpace(applicationID), strings.TrimSpace(environmentID)}
	if strings.TrimSpace(serviceID) != "" {
		query += ` AND service_id=?`
		args = append(args, strings.TrimSpace(serviceID))
	}
	query += ` ORDER BY CASE kind WHEN 'custom' THEN 0 ELSE 1 END,domain_name`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list service domains: %w", err)
	}
	defer rows.Close()
	out := make([]ServiceDomain, 0)
	for rows.Next() {
		item, err := scanServiceDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpdateServiceDomainName(ctx context.Context, applicationID, environmentID, id, domainName string) (ServiceDomain, error) {
	return s.UpdateServiceDomain(ctx, applicationID, environmentID, id, domainName, "")
}

func (s *Store) UpdateServiceDomain(ctx context.Context, applicationID, environmentID, id, domainName, serviceID string) (ServiceDomain, error) {
	existing, err := s.GetServiceDomain(ctx, applicationID, environmentID, id)
	if err != nil {
		return ServiceDomain{}, err
	}
	if existing.Kind == "platform" {
		return ServiceDomain{}, ErrManagedDomain
	}
	if strings.TrimSpace(serviceID) == "" {
		serviceID = existing.ServiceID
	}
	var valid int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM service_environments se
		JOIN services svc ON svc.id=se.service_id
		JOIN environments env ON env.id=se.environment_id
		WHERE svc.application_id=? AND env.application_id=? AND se.environment_id=? AND se.service_id=?`,
		strings.TrimSpace(applicationID), strings.TrimSpace(applicationID), strings.TrimSpace(environmentID), strings.TrimSpace(serviceID)).Scan(&valid); err != nil {
		return ServiceDomain{}, err
	}
	if valid == 0 {
		return ServiceDomain{}, ErrEnvironmentNotFound
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE domains SET domain_name=?,service_id=?,ssl_status=?,last_cert_message='',cert_checked_at='',updated_at=?
		WHERE id=? AND application_id=? AND environment_id=?`,
		strings.TrimSpace(domainName), strings.TrimSpace(serviceID), models.SSLStatusPending, time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(id), strings.TrimSpace(applicationID), strings.TrimSpace(environmentID))
	if err != nil {
		if isUniqueConstraint(err) {
			return ServiceDomain{}, ErrDuplicateDomain
		}
		return ServiceDomain{}, err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ServiceDomain{}, ErrDomainNotFound
	}
	return s.GetServiceDomain(ctx, applicationID, environmentID, id)
}

func (s *Store) DeleteServiceDomain(ctx context.Context, applicationID, environmentID, id string) error {
	item, err := s.GetServiceDomain(ctx, applicationID, environmentID, id)
	if err != nil {
		return err
	}
	if item.Kind == "platform" {
		return ErrManagedDomain
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM domains WHERE id=? AND application_id=? AND environment_id=?`, strings.TrimSpace(id), strings.TrimSpace(applicationID), strings.TrimSpace(environmentID))
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrDomainNotFound
	}
	return nil
}

func (s *Store) RestoreServiceDomain(ctx context.Context, item ServiceDomain) error {
	if item.Kind == "" {
		item.Kind = "custom"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO domains(id,application_id,environment_id,service_id,domain_name,kind,ssl_status,last_cert_message,cert_checked_at,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		item.ID, item.ApplicationID, item.EnvironmentID, item.ServiceID, item.DomainName, item.Kind, item.SSLStatus,
		item.LastCertMessage, item.CertCheckedAtRaw, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return fmt.Errorf("restore service domain: %w", err)
	}
	return nil
}

var platformDomainAdjectives = []string{"amber", "bright", "calm", "clear", "cool", "gentle", "golden", "lively", "quiet", "rapid", "silver", "sunny"}
var platformDomainNouns = []string{"brook", "cloud", "field", "forest", "harbor", "meadow", "orbit", "river", "sparrow", "summit", "willow", "wave"}

func randomPlatformLabel() (string, error) {
	pick := func(values []string) (string, error) {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(values))))
		if err != nil {
			return "", err
		}
		return values[index.Int64()], nil
	}
	adjective, err := pick(platformDomainAdjectives)
	if err != nil {
		return "", err
	}
	noun, err := pick(platformDomainNouns)
	if err != nil {
		return "", err
	}
	suffix := make([]byte, 2)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%x", adjective, noun, suffix), nil
}

// EnsurePlatformServiceDomain returns the stable HostForge-managed hostname for
// a service environment. It is a no-op until onboarding has a platform domain.
func (s *Store) EnsurePlatformServiceDomain(ctx context.Context, applicationID, environmentID, serviceID string) (ServiceDomain, bool, error) {
	existing, err := scanServiceDomain(s.db.QueryRowContext(ctx, `SELECT `+serviceDomainColumns+` FROM domains WHERE service_id=? AND environment_id=? AND kind='platform' LIMIT 1`, strings.TrimSpace(serviceID), strings.TrimSpace(environmentID)))
	if err == nil {
		return existing, false, nil
	}
	if err != sql.ErrNoRows {
		return ServiceDomain{}, false, err
	}
	state, err := s.GetOnboardingState(ctx)
	if err != nil {
		return ServiceDomain{}, false, err
	}
	platformDomain := strings.ToLower(strings.TrimSpace(state.PlatformDomain))
	if platformDomain == "" {
		return ServiceDomain{}, false, nil
	}
	var valid int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM service_environments se
		JOIN services svc ON svc.id=se.service_id
		JOIN environments env ON env.id=se.environment_id
		WHERE svc.application_id=? AND env.application_id=? AND se.environment_id=? AND se.service_id=?`,
		strings.TrimSpace(applicationID), strings.TrimSpace(applicationID), strings.TrimSpace(environmentID), strings.TrimSpace(serviceID)).Scan(&valid); err != nil {
		return ServiceDomain{}, false, err
	}
	if valid == 0 {
		return ServiceDomain{}, false, ErrEnvironmentNotFound
	}
	for attempt := 0; attempt < 12; attempt++ {
		label, err := randomPlatformLabel()
		if err != nil {
			return ServiceDomain{}, false, err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		id := newID()
		domainName := label + "." + platformDomain
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO domains(id,application_id,environment_id,service_id,domain_name,kind,ssl_status,last_cert_message,cert_checked_at,created_at,updated_at)
			VALUES(?,?,?,?,?,'platform',?,?,?,?,?)`,
			id, strings.TrimSpace(applicationID), strings.TrimSpace(environmentID), strings.TrimSpace(serviceID), domainName,
			models.SSLStatusPending, "", "", now, now)
		if err == nil {
			item, lookupErr := s.GetServiceDomain(ctx, applicationID, environmentID, id)
			return item, true, lookupErr
		}
		if !isUniqueConstraint(err) {
			return ServiceDomain{}, false, err
		}
		if existing, lookupErr := scanServiceDomain(s.db.QueryRowContext(ctx, `SELECT `+serviceDomainColumns+` FROM domains WHERE service_id=? AND environment_id=? AND kind='platform' LIMIT 1`, serviceID, environmentID)); lookupErr == nil {
			return existing, false, nil
		}
	}
	return ServiceDomain{}, false, fmt.Errorf("generate unique platform domain")
}

// EnsureActivePlatformServiceDomains backfills managed share URLs for releases
// that were already active when the platform-domain feature was enabled.
func (s *Store) EnsureActivePlatformServiceDomains(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT svc.application_id,se.environment_id,se.service_id
		FROM service_environments se
		JOIN services svc ON svc.id=se.service_id
		WHERE se.active_deployment_id<>''`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type target struct{ applicationID, environmentID, serviceID string }
	targets := make([]target, 0)
	for rows.Next() {
		var item target
		if err := rows.Scan(&item.applicationID, &item.environmentID, &item.serviceID); err != nil {
			return 0, err
		}
		targets = append(targets, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	created := 0
	for _, item := range targets {
		_, wasCreated, err := s.EnsurePlatformServiceDomain(ctx, item.applicationID, item.environmentID, item.serviceID)
		if err != nil {
			return created, err
		}
		if wasCreated {
			created++
		}
	}
	return created, nil
}
