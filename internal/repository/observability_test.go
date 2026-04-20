package repository

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/hostforge/hostforge/internal/database"
	"github.com/hostforge/hostforge/internal/models"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) (*sql.DB, *Store) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "obs.db")
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000", filepath.ToSlash(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db, New(db)
}

func TestInsertDeployStepAndListByDeployment(t *testing.T) {
	ctx := context.Background()
	_, store := openTestDB(t)
	start := time.Now().UTC().Add(-2 * time.Minute)
	end := start.Add(5 * time.Second)
	if err := store.InsertDeployStep(ctx, models.DeployStepRecord{
		DeploymentID: "dep-a",
		ProjectID:    "proj-1",
		RequestID:    "req-1",
		Step:         "clone",
		Status:       "ok",
		DurationMS:   1200,
		ErrorCode:    "",
		StartedAt:    start,
		EndedAt:      end,
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListDeployStepsByDeployment(ctx, "dep-a", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].Step != "clone" || rows[0].DurationMS != 1200 {
		t.Fatalf("unexpected row: %+v", rows[0])
	}
}

func TestSummarizeObservabilityHTTPPercentiles(t *testing.T) {
	ctx := context.Background()
	_, store := openTestDB(t)
	base := time.Now().UTC().Add(-1 * time.Hour)
	for i, ms := range []int64{10, 20, 30, 40, 100} {
		if err := store.InsertHTTPRequest(ctx, models.HTTPRequestRecord{
			RequestID:  fmt.Sprintf("r%d", i),
			Method:     "GET",
			Path:       "/api/x",
			Status:     200,
			DurationMS: ms,
			StartedAt:  base.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	sum, err := store.SummarizeObservability(ctx, 24)
	if err != nil {
		t.Fatal(err)
	}
	if sum.HTTPRequestCount != 5 {
		t.Fatalf("http count: %d", sum.HTTPRequestCount)
	}
	if sum.HTTPDurationP50 == 0 || sum.HTTPDurationP95 == 0 {
		t.Fatalf("expected non-zero percentiles, p50=%d p95=%d", sum.HTTPDurationP50, sum.HTTPDurationP95)
	}
}
