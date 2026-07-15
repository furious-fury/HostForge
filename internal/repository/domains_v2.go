package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/models"
)

type ServiceDomain struct {
	ID               string `json:"id"`
	ApplicationID    string `json:"application_id"`
	EnvironmentID    string `json:"environment_id"`
	ServiceID        string `json:"service_id"`
	DomainName       string `json:"domain_name"`
	SSLStatus        string `json:"ssl_status"`
	LastCertMessage  string `json:"last_cert_message,omitempty"`
	CertCheckedAtRaw string `json:"cert_checked_at,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func scanServiceDomain(scanner interface{ Scan(...any) error }) (ServiceDomain, error) {
	var item ServiceDomain
	err := scanner.Scan(&item.ID, &item.ApplicationID, &item.EnvironmentID, &item.ServiceID, &item.DomainName, &item.SSLStatus, &item.LastCertMessage, &item.CertCheckedAtRaw, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

const serviceDomainColumns = `id,application_id,environment_id,service_id,domain_name,ssl_status,last_cert_message,cert_checked_at,created_at,updated_at`

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
		INSERT INTO domains(id,application_id,environment_id,service_id,domain_name,ssl_status,last_cert_message,cert_checked_at,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
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
	query += ` ORDER BY domain_name`
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
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO domains(id,application_id,environment_id,service_id,domain_name,ssl_status,last_cert_message,cert_checked_at,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		item.ID, item.ApplicationID, item.EnvironmentID, item.ServiceID, item.DomainName, item.SSLStatus,
		item.LastCertMessage, item.CertCheckedAtRaw, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return fmt.Errorf("restore service domain: %w", err)
	}
	return nil
}
