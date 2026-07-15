package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const maxEnvironmentVariablesPerScope = 100

type EnvironmentVariableSealed struct {
	Key     string
	ValueCT []byte
}

type EnvironmentVariableMeta struct {
	ID            string `json:"id"`
	ApplicationID string `json:"application_id"`
	EnvironmentID string `json:"environment_id"`
	ServiceID     string `json:"service_id,omitempty"`
	Key           string `json:"key"`
	ValueLast4    string `json:"value_last4"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func (s *Store) ListEnvironmentVariablesSealed(ctx context.Context, applicationID, environmentID, serviceID string) ([]EnvironmentVariableSealed, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT key,value_ct
		FROM environment_variables
		WHERE application_id=? AND environment_id=? AND (service_id IS NULL OR service_id=?)
		ORDER BY CASE WHEN service_id IS NULL THEN 0 ELSE 1 END, key`,
		strings.TrimSpace(applicationID), strings.TrimSpace(environmentID), strings.TrimSpace(serviceID))
	if err != nil {
		return nil, fmt.Errorf("list environment variables: %w", err)
	}
	defer rows.Close()
	byKey := map[string]EnvironmentVariableSealed{}
	var order []string
	for rows.Next() {
		var item EnvironmentVariableSealed
		if err := rows.Scan(&item.Key, &item.ValueCT); err != nil {
			return nil, err
		}
		if _, exists := byKey[item.Key]; !exists {
			order = append(order, item.Key)
		}
		byKey[item.Key] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]EnvironmentVariableSealed, 0, len(order))
	for _, key := range order {
		out = append(out, byKey[key])
	}
	return out, nil
}

func scanEnvironmentVariable(scanner interface{ Scan(...any) error }) (EnvironmentVariableMeta, error) {
	var item EnvironmentVariableMeta
	var serviceID sql.NullString
	err := scanner.Scan(&item.ID, &item.ApplicationID, &item.EnvironmentID, &serviceID, &item.Key, &item.ValueLast4, &item.CreatedAt, &item.UpdatedAt)
	if serviceID.Valid {
		item.ServiceID = serviceID.String
	}
	return item, err
}

func (s *Store) ListEnvironmentVariableMeta(ctx context.Context, applicationID, environmentID, serviceID string) ([]EnvironmentVariableMeta, error) {
	query := `
		SELECT id,application_id,environment_id,service_id,key,value_last4,created_at,updated_at
		FROM environment_variables
		WHERE application_id=? AND environment_id=?`
	args := []any{strings.TrimSpace(applicationID), strings.TrimSpace(environmentID)}
	if strings.TrimSpace(serviceID) != "" {
		query += ` AND service_id=?`
		args = append(args, strings.TrimSpace(serviceID))
	} else {
		query += ` AND service_id IS NULL`
	}
	query += ` ORDER BY key`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list environment variable metadata: %w", err)
	}
	defer rows.Close()
	out := make([]EnvironmentVariableMeta, 0)
	for rows.Next() {
		item, err := scanEnvironmentVariable(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertEnvironmentVariable(ctx context.Context, applicationID, environmentID, serviceID, key string, valueCT []byte, valueLast4 string) (EnvironmentVariableMeta, error) {
	applicationID = strings.TrimSpace(applicationID)
	environmentID = strings.TrimSpace(environmentID)
	serviceID = strings.TrimSpace(serviceID)
	key = strings.TrimSpace(key)
	now := time.Now().UTC().Format(time.RFC3339)
	var existingID string
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM environment_variables
		WHERE application_id=? AND environment_id=? AND key=?
		AND ((service_id IS NULL AND ?='') OR service_id=?)`,
		applicationID, environmentID, key, serviceID, serviceID).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return EnvironmentVariableMeta{}, fmt.Errorf("lookup environment variable: %w", err)
	}
	if err == nil {
		_, err = s.db.ExecContext(ctx, `UPDATE environment_variables SET value_ct=?,value_last4=?,updated_at=? WHERE id=?`, valueCT, valueLast4, now, existingID)
		if err != nil {
			return EnvironmentVariableMeta{}, fmt.Errorf("update environment variable: %w", err)
		}
		return s.GetEnvironmentVariableMeta(ctx, applicationID, environmentID, existingID)
	}
	var count int
	countQuery := `SELECT COUNT(1) FROM environment_variables WHERE environment_id=? AND service_id IS NULL`
	countArgs := []any{environmentID}
	if serviceID != "" {
		countQuery = `SELECT COUNT(1) FROM environment_variables WHERE environment_id=? AND service_id=?`
		countArgs = append(countArgs, serviceID)
	}
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&count); err != nil {
		return EnvironmentVariableMeta{}, err
	}
	if count >= maxEnvironmentVariablesPerScope {
		return EnvironmentVariableMeta{}, ErrEnvironmentVariableLimitExceeded
	}
	id := newID()
	var nullableService any
	if serviceID != "" {
		nullableService = serviceID
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO environment_variables(id,application_id,environment_id,service_id,key,value_ct,value_last4,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, id, applicationID, environmentID, nullableService, key, valueCT, valueLast4, now, now)
	if err != nil {
		return EnvironmentVariableMeta{}, fmt.Errorf("insert environment variable: %w", err)
	}
	return s.GetEnvironmentVariableMeta(ctx, applicationID, environmentID, id)
}

func (s *Store) GetEnvironmentVariableMeta(ctx context.Context, applicationID, environmentID, id string) (EnvironmentVariableMeta, error) {
	return scanEnvironmentVariable(s.db.QueryRowContext(ctx, `
		SELECT id,application_id,environment_id,service_id,key,value_last4,created_at,updated_at
		FROM environment_variables WHERE id=? AND application_id=? AND environment_id=?`,
		strings.TrimSpace(id), strings.TrimSpace(applicationID), strings.TrimSpace(environmentID)))
}

func (s *Store) UpdateEnvironmentVariableValue(ctx context.Context, applicationID, environmentID, id string, valueCT []byte, valueLast4 string) (EnvironmentVariableMeta, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE environment_variables SET value_ct=?,value_last4=?,updated_at=?
		WHERE id=? AND application_id=? AND environment_id=?`,
		valueCT, valueLast4, time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(id), strings.TrimSpace(applicationID), strings.TrimSpace(environmentID))
	if err != nil {
		return EnvironmentVariableMeta{}, fmt.Errorf("update environment variable value: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return EnvironmentVariableMeta{}, sql.ErrNoRows
	}
	return s.GetEnvironmentVariableMeta(ctx, applicationID, environmentID, id)
}

func (s *Store) DeleteEnvironmentVariable(ctx context.Context, applicationID, environmentID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM environment_variables WHERE id=? AND application_id=? AND environment_id=?`, strings.TrimSpace(id), strings.TrimSpace(applicationID), strings.TrimSpace(environmentID))
	if err != nil {
		return fmt.Errorf("delete environment variable: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}
