package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/repository"
)

const serviceMetricSampleInterval = 10 * time.Second

type serviceMetricStore interface {
	ListActiveServiceMetricTargets(context.Context) ([]repository.ActiveServiceMetricTarget, error)
	InsertServiceMetricSample(context.Context, repository.ServiceMetricSample) (repository.ServiceMetricSample, error)
}

type containerMetricReader func(context.Context, string) (docker.ContainerMetric, error)

func startServiceMetricSampler(ctx context.Context, log *slog.Logger, store serviceMetricStore) {
	go func() {
		collectServiceMetricCycle(ctx, log, store, nil)
		ticker := time.NewTicker(serviceMetricSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectServiceMetricCycle(ctx, log, store, nil)
			}
		}
	}()
}

func collectServiceMetricCycle(ctx context.Context, log *slog.Logger, store serviceMetricStore, read containerMetricReader) int {
	targets, err := store.ListActiveServiceMetricTargets(ctx)
	if err != nil {
		log.Warn("service metric target lookup failed", "error", err)
		return 0
	}
	if len(targets) == 0 {
		return 0
	}
	if read == nil {
		client, clientErr := docker.NewClient(ctx)
		if clientErr != nil {
			log.Warn("service metric collection unavailable", "error", clientErr)
			return 0
		}
		defer client.Close()
		read = func(ctx context.Context, containerID string) (docker.ContainerMetric, error) {
			return docker.CollectContainerMetric(ctx, client, containerID)
		}
	}
	collected := 0
	for _, target := range targets {
		metric, metricErr := read(ctx, target.DockerContainerID)
		if metricErr != nil {
			log.Debug("service metric collection failed", "service_id", target.ServiceID, "environment_id", target.EnvironmentID, "error", metricErr)
			continue
		}
		_, persistErr := store.InsertServiceMetricSample(ctx, repository.ServiceMetricSample{
			ServiceID: target.ServiceID, EnvironmentID: target.EnvironmentID,
			CPUPercent: metric.CPUPercent, MemoryBytes: metric.MemoryBytes,
			NetworkRXBytes: metric.NetworkRX, NetworkTXBytes: metric.NetworkTX,
			SampledAt: metric.SampledAt.UTC().Format(time.RFC3339Nano),
		})
		if persistErr != nil {
			log.Warn("service metric persistence failed", "service_id", target.ServiceID, "environment_id", target.EnvironmentID, "error", persistErr)
			continue
		}
		collected++
	}
	return collected
}
