package main

import (
	"net/http"
	"strconv"

	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/repository"
	platformservices "github.com/hostforge/hostforge/internal/services"
)

func (s *server) handleServiceMetricsV2(w http.ResponseWriter, r *http.Request, serviceID, environmentID string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	points := 120
	if value, err := strconv.Atoi(r.URL.Query().Get("points")); err == nil && value > 0 && value <= 720 {
		points = value
	}
	target, deployment, container, err := platformservices.ResolveActiveServiceContainer(r.Context(), s.store, serviceID, environmentID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"supported": false, "error_code": platformservices.FirstPublicCode(err), "samples": []repository.ServiceMetricSample{}})
		return
	}
	if target.Binding.DesiredState == "stopped" {
		samples, _ := s.store.ListServiceMetricSamples(r.Context(), serviceID, environmentID, points)
		writeJSON(w, http.StatusOK, map[string]any{"supported": true, "stale": true, "deployment_id": deployment.ID, "samples": samples})
		return
	}
	client, err := docker.NewClient(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "docker_unavailable"})
		return
	}
	defer client.Close()
	metric, err := docker.CollectContainerMetric(r.Context(), client, container.DockerContainerID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": "container_metrics_failed"})
		return
	}
	current, err := s.store.InsertServiceMetricSample(r.Context(), repository.ServiceMetricSample{
		ServiceID: serviceID, EnvironmentID: environmentID, CPUPercent: metric.CPUPercent,
		MemoryBytes: metric.MemoryBytes, NetworkRXBytes: metric.NetworkRX, NetworkTXBytes: metric.NetworkTX,
		SampledAt: metric.SampledAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "persist_service_metrics_failed"})
		return
	}
	samples, err := s.store.ListServiceMetricSamples(r.Context(), serviceID, environmentID, points)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_service_metrics_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"supported": true, "stale": false, "deployment_id": deployment.ID, "sample": current, "samples": samples})
}
