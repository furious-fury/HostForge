package main

import (
	"context"
	"net/http"
	"runtime"

	"github.com/hostforge/hostforge/internal/databases"
	"github.com/hostforge/hostforge/internal/hostmetrics"
	"github.com/hostforge/hostforge/internal/repository"
)

const (
	databaseMinimumCPUReserveMillis = 250
	databaseMinimumMemoryReserve    = int64(512 * 1024 * 1024)
)

type databaseResourceCapacity struct {
	Available            bool  `json:"available"`
	CPUTotalMillis       int   `json:"cpu_total_millis"`
	CPUAllocatedMillis   int   `json:"cpu_allocated_millis"`
	CPUReserveMillis     int   `json:"cpu_reserve_millis"`
	CPUAvailableMillis   int   `json:"cpu_available_millis"`
	MemoryTotalBytes     int64 `json:"memory_total_bytes"`
	MemoryAllocatedBytes int64 `json:"memory_allocated_bytes"`
	MemoryReserveBytes   int64 `json:"memory_reserve_bytes"`
	MemoryAvailableBytes int64 `json:"memory_available_bytes"`
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func calculateManagedDatabaseCapacity(sample hostmetrics.Sample, instances []repository.DatabaseInstance, cpuTotalMillis int) databaseResourceCapacity {
	capacity := databaseResourceCapacity{Available: true, CPUTotalMillis: cpuTotalMillis, MemoryTotalBytes: sample.Mem.TotalBytes}
	for _, instance := range instances {
		if !instance.DeletedAt.IsZero() || instance.DesiredState == "deleted" {
			continue
		}
		capacity.CPUAllocatedMillis += instance.CPULimitMillis
		capacity.MemoryAllocatedBytes += instance.MemoryLimitBytes
	}
	capacity.CPUReserveMillis = max(databaseMinimumCPUReserveMillis, capacity.CPUTotalMillis/10)
	capacity.CPUAvailableMillis = max(0, capacity.CPUTotalMillis-capacity.CPUReserveMillis-capacity.CPUAllocatedMillis)
	capacity.MemoryReserveBytes = maxInt64(databaseMinimumMemoryReserve, sample.Mem.TotalBytes/10)
	allocationHeadroom := maxInt64(0, sample.Mem.TotalBytes-capacity.MemoryReserveBytes-capacity.MemoryAllocatedBytes)
	liveHeadroom := maxInt64(0, sample.Mem.AvailableBytes-capacity.MemoryReserveBytes)
	capacity.MemoryAvailableBytes = minInt64(allocationHeadroom, liveHeadroom)
	return capacity
}

func (s *server) managedDatabaseCapacity(ctx context.Context) databaseResourceCapacity {
	capacity := databaseResourceCapacity{}
	if s.hostSampler == nil || !s.hostSampler.Supported() || !s.hostSampler.HasSamples() || s.store == nil {
		return capacity
	}
	sample := s.hostSampler.Latest()
	if sample.Mem.TotalBytes <= 0 || sample.Mem.AvailableBytes <= 0 {
		return capacity
	}
	instances, err := s.store.ListAllDatabaseInstances(ctx)
	if err != nil {
		return capacity
	}
	return calculateManagedDatabaseCapacity(sample, instances, runtime.NumCPU()*1000)
}

func (s *server) handleDatabaseEngines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "error", "error": "method_not_allowed"})
		return
	}
	gatewayEnabled := s.cfg != nil && s.cfg.DatabaseGatewaysEnabled
	engines := databases.Catalog()
	for index := range engines {
		if engines[index].ID == "postgresql" {
			engines[index].PublicAccessAvailable = gatewayEnabled
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"engines":           engines,
		"resource_presets":  databases.ResourcePresets(),
		"resource_capacity": s.managedDatabaseCapacity(r.Context()),
		"networking": map[string]any{
			"scope":                   "hostforge_environment",
			"public_access_available": gatewayEnabled,
		},
	})
}
