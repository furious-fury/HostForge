package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hostforge/hostforge/internal/hostmetrics"
	"github.com/hostforge/hostforge/internal/repository"
)

func TestManagedDatabaseCapacityReservesHostAndExistingAllocations(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)
	sample := hostmetrics.Sample{Mem: hostmetrics.MemSample{TotalBytes: 8 * gib, AvailableBytes: 6 * gib}}
	instances := []repository.DatabaseInstance{
		{CPULimitMillis: 1000, MemoryLimitBytes: 2 * gib, DesiredState: "running"},
		{CPULimitMillis: 2000, MemoryLimitBytes: 4 * gib, DesiredState: "deleted", DeletedAt: time.Now()},
	}
	capacity := calculateManagedDatabaseCapacity(sample, instances, 4000)
	if !capacity.Available || capacity.CPUReserveMillis != 400 || capacity.CPUAvailableMillis != 2600 {
		t.Fatalf("unexpected CPU capacity: %+v", capacity)
	}
	expectedMemoryReserve := 8 * gib / 10
	if capacity.MemoryReserveBytes != expectedMemoryReserve || capacity.MemoryAvailableBytes != 6*gib-expectedMemoryReserve {
		t.Fatalf("unexpected memory capacity: %+v", capacity)
	}
}

func TestDatabaseEngineCatalogContract(t *testing.T) {
	recorder := httptest.NewRecorder()
	(&server{}).handleDatabaseEngines(recorder, httptest.NewRequest(http.MethodGet, "/api/database-engines", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Engines []struct {
			ID                    string `json:"id"`
			PublicAccessAvailable bool   `json:"public_access_available"`
		} `json:"engines"`
		ResourcePresets []struct {
			ID string `json:"id"`
		} `json:"resource_presets"`
		ResourceCapacity struct {
			Available bool `json:"available"`
		} `json:"resource_capacity"`
		Networking struct {
			Scope                 string `json:"scope"`
			PublicAccessAvailable bool   `json:"public_access_available"`
		} `json:"networking"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Engines) != 6 || len(payload.ResourcePresets) != 4 {
		t.Fatalf("engines=%d presets=%d", len(payload.Engines), len(payload.ResourcePresets))
	}
	if payload.Networking.Scope != "hostforge_environment" || payload.Networking.PublicAccessAvailable {
		t.Fatalf("unexpected networking contract: %+v", payload.Networking)
	}
	if payload.ResourceCapacity.Available {
		t.Fatal("capacity should be unavailable without a host sampler")
	}
	for _, engine := range payload.Engines {
		if engine.PublicAccessAvailable {
			t.Fatalf("public access enabled for %s", engine.ID)
		}
	}
}
