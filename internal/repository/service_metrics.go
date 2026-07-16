package repository

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const serviceMetricRetentionPerBinding = 720

type ActiveServiceMetricTarget struct {
	ServiceID         string
	EnvironmentID     string
	DeploymentID      string
	DockerContainerID string
}

type ServiceMetricSample struct {
	ID             int64   `json:"id"`
	ServiceID      string  `json:"service_id"`
	EnvironmentID  string  `json:"environment_id"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryBytes    int64   `json:"memory_bytes"`
	NetworkRXBytes int64   `json:"network_rx_bytes"`
	NetworkTXBytes int64   `json:"network_tx_bytes"`
	SampledAt      string  `json:"sampled_at"`
}

func (s *Store) ListActiveServiceMetricTargets(ctx context.Context) ([]ActiveServiceMetricTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT se.service_id,se.environment_id,se.active_deployment_id,c.docker_container_id
		FROM service_environments se
		JOIN containers c ON c.id=(
			SELECT latest.id FROM containers latest
			WHERE latest.deployment_id=se.active_deployment_id
			ORDER BY latest.created_at DESC,latest.id DESC LIMIT 1
		)
		WHERE se.desired_state='running'
		  AND se.active_deployment_id<>''
		  AND c.status='RUNNING'
		ORDER BY se.service_id,se.environment_id`)
	if err != nil {
		return nil, fmt.Errorf("list active service metric targets: %w", err)
	}
	defer rows.Close()
	targets := make([]ActiveServiceMetricTarget, 0)
	for rows.Next() {
		var target ActiveServiceMetricTarget
		if err := rows.Scan(&target.ServiceID, &target.EnvironmentID, &target.DeploymentID, &target.DockerContainerID); err != nil {
			return nil, fmt.Errorf("scan active service metric target: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active service metric targets: %w", err)
	}
	return targets, nil
}

func (s *Store) InsertServiceMetricSample(ctx context.Context, sample ServiceMetricSample) (ServiceMetricSample, error) {
	if sample.SampledAt == "" {
		sample.SampledAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO service_metric_samples(service_id,environment_id,cpu_percent,memory_bytes,network_rx_bytes,network_tx_bytes,sampled_at)
		VALUES(?,?,?,?,?,?,?)`,
		strings.TrimSpace(sample.ServiceID), strings.TrimSpace(sample.EnvironmentID), sample.CPUPercent, sample.MemoryBytes, sample.NetworkRXBytes, sample.NetworkTXBytes, sample.SampledAt)
	if err != nil {
		return ServiceMetricSample{}, fmt.Errorf("insert service metric: %w", err)
	}
	sample.ID, _ = result.LastInsertId()
	_, err = s.db.ExecContext(ctx, `
		DELETE FROM service_metric_samples
		WHERE service_id=? AND environment_id=? AND id NOT IN (
			SELECT id FROM service_metric_samples WHERE service_id=? AND environment_id=? ORDER BY id DESC LIMIT ?
		)`, sample.ServiceID, sample.EnvironmentID, sample.ServiceID, sample.EnvironmentID, serviceMetricRetentionPerBinding)
	if err != nil {
		return ServiceMetricSample{}, fmt.Errorf("trim service metrics: %w", err)
	}
	return sample, nil
}

func (s *Store) ListServiceMetricSamples(ctx context.Context, serviceID, environmentID string, limit int) ([]ServiceMetricSample, error) {
	if limit <= 0 || limit > serviceMetricRetentionPerBinding {
		limit = 120
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,service_id,environment_id,cpu_percent,memory_bytes,network_rx_bytes,network_tx_bytes,sampled_at
		FROM service_metric_samples WHERE service_id=? AND environment_id=? ORDER BY id DESC LIMIT ?`,
		strings.TrimSpace(serviceID), strings.TrimSpace(environmentID), limit)
	if err != nil {
		return nil, fmt.Errorf("list service metrics: %w", err)
	}
	defer rows.Close()
	out := make([]ServiceMetricSample, 0)
	for rows.Next() {
		var item ServiceMetricSample
		if err := rows.Scan(&item.ID, &item.ServiceID, &item.EnvironmentID, &item.CPUPercent, &item.MemoryBytes, &item.NetworkRXBytes, &item.NetworkTXBytes, &item.SampledAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out, nil
}
