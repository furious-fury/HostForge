package repository

import (
	"context"
	"fmt"
	"testing"
)

func TestServiceMetricSamplesAreScopedOrderedAndBounded(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < serviceMetricRetentionPerBinding+5; i++ {
		_, err := store.InsertServiceMetricSample(ctx, ServiceMetricSample{ServiceID: "svc", EnvironmentID: "prod", CPUPercent: float64(i), SampledAt: fmt.Sprintf("2026-01-01T00:%02d:00Z", i%60)})
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.ListServiceMetricSamples(ctx, "svc", "prod", serviceMetricRetentionPerBinding)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != serviceMetricRetentionPerBinding {
		t.Fatalf("expected bounded retention, got %d", len(rows))
	}
	if rows[0].CPUPercent != 5 || rows[len(rows)-1].CPUPercent != float64(serviceMetricRetentionPerBinding+4) {
		t.Fatalf("samples not returned oldest to newest after trim: first=%v last=%v", rows[0].CPUPercent, rows[len(rows)-1].CPUPercent)
	}
	other, err := store.ListServiceMetricSamples(ctx, "svc", "staging", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("scope leaked: rows=%v err=%v", other, err)
	}
}
