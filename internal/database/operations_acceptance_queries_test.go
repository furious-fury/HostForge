package database

import "testing"

// scripts/operations-queue-acceptance.sh reads the control-plane database
// directly. Nothing else compiles or type-checks those queries, so a renamed
// column would break the acceptance run silently — and it would only be
// discovered on the VPS, during the upgrade it exists to verify.
//
// These tests keep the script honest: the first proves every query still
// parses against the real schema, the second proves the checks actually fire
// on the faults they describe rather than returning zero because they match
// nothing.
//
// Keep them in step with the script.

// acceptanceQueries mirrors the read-only checks in sections A-D.
var acceptanceQueries = map[string]string{
	"table exists":        `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='operations'`,
	"queue rows":          `SELECT COUNT(*) FROM operations`,
	"projection rows":     `SELECT COUNT(*) FROM database_operations`,
	"orphaned projection": `SELECT COUNT(*) FROM database_operations d WHERE NOT EXISTS (SELECT 1 FROM operations o WHERE o.id=d.id)`,
	"orphaned queue":      `SELECT COUNT(*) FROM operations o WHERE NOT EXISTS (SELECT 1 FROM database_operations d WHERE d.id=o.id)`,
	"bad lock keys":       `SELECT COUNT(*) FROM operations WHERE lock_key='' OR lock_key IS NULL OR (lock_key NOT LIKE 'dbi:%' AND lock_key NOT LIKE 'dbsvc:%')`,
	"delete rows":         `SELECT COUNT(*) FROM database_operations WHERE operation_type='delete'`,
	"mismatched delete":   `SELECT COUNT(*) FROM operations o JOIN database_operations d USING(id) WHERE d.operation_type='delete' AND o.lock_key NOT LIKE 'dbsvc:%'`,
	"diverged": `SELECT COUNT(*) FROM operations o JOIN database_operations d USING(id)
		WHERE o.status <> d.status OR o.progress_percent <> d.progress_percent OR o.attempt <> d.attempt_count`,
	"invisible":            `SELECT COUNT(*) FROM operations WHERE status='queued' AND attempt>=max_attempts`,
	"expired":              `SELECT COUNT(*) FROM operations WHERE status='running' AND lease_expires_at<>'' AND lease_expires_at <= strftime('%Y-%m-%dT%H:%M:%SZ','now')`,
	"running for lock key": `SELECT COUNT(*) FROM operations WHERE lock_key='dbi:x' AND status='running'`,
	"instance picker":      `SELECT id,network_alias,status FROM database_instances WHERE deleted_at=''`,
}

func TestAcceptanceScriptQueriesMatchTheSchema(t *testing.T) {
	t.Parallel()
	db, ctx := openFullyMigratedDB(t, "acceptance-queries.db")

	for name, query := range acceptanceQueries {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			t.Errorf("acceptance query %q no longer matches the schema: %v", name, err)
			continue
		}
		rows.Close()
	}
}

func TestAcceptanceChecksDetectSeededFaults(t *testing.T) {
	t.Parallel()
	db, ctx := openFullyMigratedDB(t, "acceptance-detect.db")

	seed := func(id, lockKey, status string, attempt, maxAttempts int, lease string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `
			INSERT INTO operations(id,kind,lock_key,status,attempt,max_attempts,lease_expires_at,created_at,updated_at)
			VALUES(?,'db_backup',?,?,?,?,?,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`,
			id, lockKey, status, attempt, maxAttempts, lease); err != nil {
			t.Fatal(err)
		}
	}
	count := func(query string) int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// One row per fault the script reports on.
	seed("orphan-1", "dbi:i1", "queued", 0, 5, "")                       // no projection row
	seed("badkey-1", "nonsense", "queued", 0, 5, "")                     // malformed lock key
	seed("stuck-1", "dbi:i2", "queued", 5, 5, "")                        // queued past its attempt limit
	seed("expired-1", "dbi:i3", "running", 1, 5, "2020-01-01T00:00:00Z") // running on a dead lease

	for _, tc := range []struct {
		name        string
		wantAtLeast int
	}{
		{"orphaned queue", 4},
		{"bad lock keys", 1},
		{"invisible", 1},
		{"expired", 1},
	} {
		if got := count(acceptanceQueries[tc.name]); got < tc.wantAtLeast {
			t.Errorf("the %q check found %d, want at least %d: it does not detect the fault it reports on",
				tc.name, got, tc.wantAtLeast)
		}
	}
}
