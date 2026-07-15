package repository

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type PlatformEvent struct {
	ID            int64  `json:"id"`
	ApplicationID string `json:"application_id,omitempty"`
	ServiceID     string `json:"service_id,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	DeploymentID  string `json:"deployment_id,omitempty"`
	EventType     string `json:"event_type"`
	Status        string `json:"status,omitempty"`
	Actor         string `json:"actor,omitempty"`
	Message       string `json:"message"`
	Detail        string `json:"detail,omitempty"`
	CreatedAt     string `json:"created_at"`
}

type PlatformEventInput struct {
	ApplicationID string
	ServiceID     string
	EnvironmentID string
	DeploymentID  string
	EventType     string
	Status        string
	Actor         string
	Message       string
	Detail        string
}

func (s *Store) RecordPlatformEvent(ctx context.Context, input PlatformEventInput) error {
	if strings.TrimSpace(input.EventType) == "" || strings.TrimSpace(input.Message) == "" {
		return fmt.Errorf("event type and message are required")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO platform_events(application_id,service_id,environment_id,deployment_id,event_type,status,actor,message,detail,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		strings.TrimSpace(input.ApplicationID), strings.TrimSpace(input.ServiceID), strings.TrimSpace(input.EnvironmentID),
		strings.TrimSpace(input.DeploymentID), strings.TrimSpace(input.EventType), strings.TrimSpace(input.Status),
		strings.TrimSpace(input.Actor), strings.TrimSpace(input.Message), strings.TrimSpace(input.Detail),
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("record platform event: %w", err)
	}
	return nil
}

func (s *Store) ListPlatformEvents(ctx context.Context, applicationID, serviceID, eventType string, limit int) ([]PlatformEvent, error) {
	items, _, err := s.ListPlatformEventsFiltered(ctx, PlatformEventFilter{ApplicationID: applicationID, ServiceID: serviceID, EventType: eventType, Limit: limit})
	return items, err
}

type PlatformEventFilter struct {
	ApplicationID string
	ServiceID     string
	EventType     string
	DateFrom      string
	DateTo        string
	Cursor        int64
	Limit         int
}

func (s *Store) ListPlatformEventsFiltered(ctx context.Context, filter PlatformEventFilter) ([]PlatformEvent, int64, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT id,application_id,service_id,environment_id,deployment_id,event_type,status,actor,message,detail,created_at
		FROM platform_events
		WHERE 1=1`
	args := make([]any, 0, 7)
	add := func(clause string, value any) { query += " AND " + clause; args = append(args, value) }
	if value := strings.TrimSpace(filter.ApplicationID); value != "" {
		add("application_id=?", value)
	}
	if value := strings.TrimSpace(filter.ServiceID); value != "" {
		add("service_id=?", value)
	}
	if value := strings.TrimSpace(filter.EventType); value != "" {
		add("event_type=?", value)
	}
	if value := strings.TrimSpace(filter.DateFrom); value != "" {
		add("created_at>=?", value)
	}
	if value := strings.TrimSpace(filter.DateTo); value != "" {
		add("created_at<=?", value)
	}
	if filter.Cursor > 0 {
		add("id<?", filter.Cursor)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list platform events: %w", err)
	}
	defer rows.Close()
	out := make([]PlatformEvent, 0, limit+1)
	for rows.Next() {
		var item PlatformEvent
		if err := rows.Scan(&item.ID, &item.ApplicationID, &item.ServiceID, &item.EnvironmentID, &item.DeploymentID, &item.EventType, &item.Status, &item.Actor, &item.Message, &item.Detail, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	next := int64(0)
	if len(out) > limit {
		next = out[limit-1].ID
		out = out[:limit]
	}
	return out, next, nil
}

func (s *Store) RecordDeploymentEvent(ctx context.Context, deploymentID, status, detail string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO platform_events(application_id,service_id,environment_id,deployment_id,event_type,status,actor,message,detail,created_at)
		SELECT COALESCE(svc.application_id,''),COALESCE(d.service_id,''),COALESCE(d.environment_id,''),d.id,
		       'deployment',?,COALESCE(d.actor,''),'Deployment '+lower(?),?,?
		FROM deployments d LEFT JOIN services svc ON svc.id=d.service_id WHERE d.id=?`,
		strings.TrimSpace(status), strings.TrimSpace(status), strings.TrimSpace(detail), time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(deploymentID))
	if err != nil {
		return fmt.Errorf("record deployment event: %w", err)
	}
	return nil
}
