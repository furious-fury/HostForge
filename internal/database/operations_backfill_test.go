package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// The 0028 backfill is the one part of this migration that behaves
// differently on a populated database than on a fresh one, so it is tested
// against rows inserted before it runs — including the two shapes that only
// exist in real data.
func TestBackfillPopulatesOperationsForExistingRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "backfill.db")
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", filepath.ToSlash(dbPath)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Migrate up to but not including 0028, then insert as the old schema.
	seedAppliedThrough(t, ctx, db, "0028_operations_queue.sql")

	stamp := "2026-01-02T03:04:05Z"
	for _, statement := range []string{
		`INSERT INTO applications(id,name,created_at,updated_at) VALUES('app-1','App',?,?)`,
		`INSERT INTO environments(id,application_id,name,slug,kind,created_at,updated_at) VALUES('env-1','app-1','Production','production','production',?,?)`,
		`INSERT INTO services(id,application_id,name,repo_url,internal_port,service_type,created_at,updated_at)
		 VALUES('svc-1','app-1','database','',5432,'database',?,?)`,
		`INSERT INTO database_instances(id,service_id,environment_id,engine_version,image_ref,network_alias,volume_name,internal_port,resource_preset,cpu_limit_millis,memory_limit_bytes,status,desired_state,created_at,updated_at)
		 VALUES('inst-1','svc-1','env-1','18','postgres@sha256:test','database','hostforge-db-1',5432,'standard',1000,1073741824,'healthy','running',?,?)`,
	} {
		if _, err := db.ExecContext(ctx, statement, stamp, stamp); err != nil {
			t.Fatalf("seed %q: %v", statement, err)
		}
	}

	// A normal instance-scoped operation, mid-flight with a live lease.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO database_operations(id,service_id,database_instance_id,operation_type,status,progress_step,progress_percent,actor,attempt_count,lease_owner,lease_expires_at,started_at,created_at,updated_at)
		VALUES('op-running','svc-1','inst-1','provision','running','creating_volume',30,'operator',2,'worker-a',?,?,?,?)`,
		stamp, stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	// The shape that only exists on a used database: a terminal 'delete'
	// audit row with a NULL instance id. Keying lock_key off the instance
	// alone produces NULL here and fails the NOT NULL constraint.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO database_operations(id,service_id,database_instance_id,operation_type,status,progress_step,progress_percent,actor,started_at,completed_at,created_at,updated_at)
		VALUES('op-delete','svc-1',NULL,'delete','success','volume_retained',100,'operator',?,?,?,?)`,
		stamp, stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("ApplyMigrations (0028): %v", err)
	}

	var operations, sources int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operations`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_operations`).Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if operations != sources {
		t.Fatalf("backfilled %d operations from %d database_operations rows", operations, sources)
	}

	for _, tc := range []struct {
		id, kind, lockKey, status, applicationID, serviceID, environmentID string
		attempt                                                            int
	}{
		{"op-running", "db_provision", "dbi:inst-1", "running", "app-1", "svc-1", "env-1", 2},
		{"op-delete", "db_delete", "dbsvc:svc-1", "success", "app-1", "svc-1", "", 0},
	} {
		var kind, lockKey, status, applicationID, serviceID, environmentID string
		var attempt int
		if err := db.QueryRowContext(ctx,
			`SELECT kind,lock_key,status,application_id,service_id,environment_id,attempt FROM operations WHERE id=?`, tc.id,
		).Scan(&kind, &lockKey, &status, &applicationID, &serviceID, &environmentID, &attempt); err != nil {
			t.Fatalf("read backfilled %s: %v", tc.id, err)
		}
		if kind != tc.kind || lockKey != tc.lockKey || status != tc.status ||
			applicationID != tc.applicationID || serviceID != tc.serviceID ||
			environmentID != tc.environmentID || attempt != tc.attempt {
			t.Errorf("%s backfilled as kind=%q lock_key=%q status=%q app=%q svc=%q env=%q attempt=%d;\n want kind=%q lock_key=%q status=%q app=%q svc=%q env=%q attempt=%d",
				tc.id, kind, lockKey, status, applicationID, serviceID, environmentID, attempt,
				tc.kind, tc.lockKey, tc.status, tc.applicationID, tc.serviceID, tc.environmentID, tc.attempt)
		}
	}

	// The lease on the in-flight row survives, so recovery can see it.
	var leaseOwner string
	if err := db.QueryRowContext(ctx, `SELECT lease_owner FROM operations WHERE id='op-running'`).Scan(&leaseOwner); err != nil {
		t.Fatal(err)
	}
	if leaseOwner != "worker-a" {
		t.Fatalf("lease_owner = %q, want worker-a", leaseOwner)
	}
}

// A fresh database has nothing to backfill; the migration must still apply
// and leave an empty, usable table.
func TestOperationsTableAppliesOnFreshDatabase(t *testing.T) {
	t.Parallel()
	db, ctx := openFullyMigratedDB(t, "operations-fresh.db")

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operations`).Scan(&count); err != nil {
		t.Fatalf("operations table unusable after a fresh migrate: %v", err)
	}
	if count != 0 {
		t.Fatalf("fresh database has %d operations rows, want 0", count)
	}
}

// The claim query the runtime will issue must not full-scan.
func TestOperationsClaimUsesAnIndex(t *testing.T) {
	t.Parallel()
	db, ctx := openFullyMigratedDB(t, "operations-claim-index.db")

	plan := explainQueryPlan(t, db, ctx, `
		SELECT o.id FROM operations o
		WHERE o.status='queued'
		  AND o.available_at<=?
		  AND o.attempt < o.max_attempts
		  AND o.priority >= ?
		  AND NOT EXISTS (SELECT 1 FROM operations r WHERE r.status='running' AND r.lock_key=o.lock_key)
		ORDER BY o.priority DESC, o.created_at, o.id
		LIMIT 1`, "2026-01-01T00:00:00Z", 0)

	for _, detail := range plan {
		if len(detail) >= 12 && (detail[:12] == "SCAN operati" || detail[:6] == "SCAN o") {
			t.Fatalf("claim query full-scans operations; plan:\n%v", plan)
		}
	}
}
