package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/furious-fury/HostForge/internal/repository"
	platformservices "github.com/furious-fury/HostForge/internal/services"
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
	target, deployment, _, err := platformservices.ResolveActiveServiceContainer(r.Context(), s.store, serviceID, environmentID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"supported": false, "error_code": platformservices.FirstPublicCode(err), "samples": []repository.ServiceMetricSample{}})
		return
	}
	if target.Binding.DesiredState == "stopped" {
		samples, listErr := s.store.ListServiceMetricSamples(r.Context(), serviceID, environmentID, points)
		if listErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_service_metrics_failed"})
			return
		}
		var current *repository.ServiceMetricSample
		if len(samples) > 0 {
			current = &samples[len(samples)-1]
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"supported": true, "stale": true, "stale_reason": "service_stopped",
			"deployment_id": deployment.ID, "sample": current, "samples": samples,
			"sample_interval_seconds": int(serviceMetricSampleInterval.Seconds()),
		})
		return
	}
	samples, err := s.store.ListServiceMetricSamples(r.Context(), serviceID, environmentID, points)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "list_service_metrics_failed"})
		return
	}
	var current *repository.ServiceMetricSample
	stale := false
	staleReason := ""
	if len(samples) > 0 {
		current = &samples[len(samples)-1]
		if sampledAt, parseErr := time.Parse(time.RFC3339Nano, current.SampledAt); parseErr == nil {
			stale = time.Since(sampledAt) > 3*serviceMetricSampleInterval
			if stale {
				staleReason = "collector_delayed"
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"supported": true, "stale": stale, "stale_reason": staleReason, "deployment_id": deployment.ID,
		"sample": current, "samples": samples, "sample_interval_seconds": int(serviceMetricSampleInterval.Seconds()),
	})
}
