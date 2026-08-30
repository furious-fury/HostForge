package database

import (
	"context"
	"path/filepath"
	"testing"
)

// The 0023 backfill repairs rows written by the `'Deployment '+lower(status)`
// bug. Its WHERE clause is the only risky part, so this asserts it rewrites the
// damaged rows and leaves everything else untouched.
func TestBackfillDeploymentEventMessagesGuard(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "backfill.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rows := []struct {
		name      string
		eventType string
		status    string
		message   string
		want      string
	}{
		{"damaged deployment row", "deployment", "SUCCESS", "0", "Deployment success"},
		{"healthy deployment row", "deployment", "FAILED", "Deployment failed", "Deployment failed"},
		{"non-deployment row that happens to say 0", "domain", "SUCCESS", "0", "0"},
		{"deployment row with no status", "deployment", "", "0", "0"},
	}
	for _, r := range rows {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO platform_events(event_type,status,message,created_at) VALUES(?,?,?,'2026-01-01T00:00:00Z')`,
			r.eventType, r.status, r.message); err != nil {
			t.Fatalf("seed %s: %v", r.name, err)
		}
	}

	body, err := migrationFiles.ReadFile("migrations/0023_backfill_deployment_event_messages.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(body)); err != nil {
		t.Fatalf("apply backfill: %v", err)
	}

	for i, r := range rows {
		var got string
		if err := db.QueryRowContext(ctx, `SELECT message FROM platform_events ORDER BY id LIMIT 1 OFFSET ?`, i).Scan(&got); err != nil {
			t.Fatalf("read back %s: %v", r.name, err)
		}
		if got != r.want {
			t.Errorf("%s: message = %q, want %q", r.name, got, r.want)
		}
	}
}
