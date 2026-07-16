package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/repository"
)

type metricSamplerStore struct {
	targets []repository.ActiveServiceMetricTarget
	samples []repository.ServiceMetricSample
}

func (s *metricSamplerStore) ListActiveServiceMetricTargets(context.Context) ([]repository.ActiveServiceMetricTarget, error) {
	return s.targets, nil
}

func (s *metricSamplerStore) InsertServiceMetricSample(_ context.Context, sample repository.ServiceMetricSample) (repository.ServiceMetricSample, error) {
	s.samples = append(s.samples, sample)
	return sample, nil
}

func TestCollectServiceMetricCycleIsolatesContainerFailures(t *testing.T) {
	store := &metricSamplerStore{targets: []repository.ActiveServiceMetricTarget{
		{ServiceID: "broken", EnvironmentID: "production", DockerContainerID: "broken-container"},
		{ServiceID: "healthy", EnvironmentID: "staging", DockerContainerID: "healthy-container"},
	}}
	read := func(_ context.Context, containerID string) (docker.ContainerMetric, error) {
		if containerID == "broken-container" {
			return docker.ContainerMetric{}, errors.New("container disappeared")
		}
		return docker.ContainerMetric{CPUPercent: 12.5, MemoryBytes: 4096, NetworkRX: 42, NetworkTX: 84, SampledAt: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}, nil
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	if count := collectServiceMetricCycle(context.Background(), log, store, read); count != 1 {
		t.Fatalf("collected=%d want=1", count)
	}
	if len(store.samples) != 1 || store.samples[0].ServiceID != "healthy" || store.samples[0].EnvironmentID != "staging" {
		t.Fatalf("unexpected samples: %+v", store.samples)
	}
	if store.samples[0].CPUPercent != 12.5 || store.samples[0].SampledAt != "2026-07-15T12:00:00Z" {
		t.Fatalf("metric fields were not preserved: %+v", store.samples[0])
	}
}
