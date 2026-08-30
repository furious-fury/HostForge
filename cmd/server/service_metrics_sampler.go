package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/repository"
	mobyclient "github.com/moby/moby/client"
)

const serviceMetricSampleInterval = 10 * time.Second

type serviceMetricStore interface {
	ListActiveServiceMetricTargets(context.Context) ([]repository.ActiveServiceMetricTarget, error)
	InsertServiceMetricSample(context.Context, repository.ServiceMetricSample) (repository.ServiceMetricSample, error)
}

type containerMetricReader func(context.Context, string) (docker.ContainerMetric, error)

func startServiceMetricSampler(ctx context.Context, log *slog.Logger, store serviceMetricStore, dockerClient *mobyclient.Client) {
	go func() {
		collectServiceMetricCycle(ctx, log, store, dockerClient, nil)
		ticker := time.NewTicker(serviceMetricSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectServiceMetricCycle(ctx, log, store, dockerClient, nil)
			}
		}
	}()
}

func collectServiceMetricCycle(ctx context.Context, log *slog.Logger, store serviceMetricStore, dockerClient *mobyclient.Client, read containerMetricReader) int {
	targets, err := store.ListActiveServiceMetricTargets(ctx)
	if err != nil {
		log.Warn("service metric target lookup failed", "error", err)
		return 0
	}
	if len(targets) == 0 {
		return 0
	}
	if read == nil {
		client := dockerClient
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
