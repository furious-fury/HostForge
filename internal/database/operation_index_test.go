package database

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// explainQueryPlan returns the detail column of EXPLAIN QUERY PLAN for query.
func explainQueryPlan(t *testing.T, db interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}, ctx context.Context, query string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var plan []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(plan) == 0 {
		t.Fatal("EXPLAIN QUERY PLAN returned no rows")
	}
	return plan
}

// assertNoFullScan fails if the plan full-scans database_operations. SQLite
// names the table by its alias when the query uses one, so both spellings
// must be checked — matching only "SCAN database_operations" silently passes
// against a plan that reads "SCAN op".
func assertNoFullScan(t *testing.T, plan []string) {
	t.Helper()
	for _, detail := range plan {
		if strings.HasPrefix(detail, "SCAN database_operations") || strings.HasPrefix(detail, "SCAN op") {
			t.Fatalf("query full-scans database_operations; plan:\n%s", strings.Join(plan, "\n"))
		}
	}
}

// The claim query runs on a 2-second poll per worker and, before 0027, had no
// supporting index: only idx_database_operations_service(service_id, ...)
// existed, which the claim does not filter on. Asserting the index exists in
// sqlite_master would not prove much — what regressed is whether the planner
// actually uses it, so assert on EXPLAIN QUERY PLAN instead.
func TestDatabaseOperationClaimUsesAnIndex(t *testing.T) {
	t.Parallel()
	db, ctx := openFullyMigratedDB(t, "claim-index.db")

	// The candidate SELECT from ClaimNextDatabaseOperation, reduced to the
	// parts the planner needs in order to choose an access path. Note this
	// query aliases the table as "op", which is why assertNoFullScan has to
	// check the alias too.
	plan := explainQueryPlan(t, db, ctx, `
		SELECT op.id FROM database_operations op
		WHERE (op.status='queued' OR (op.status='running' AND (op.lease_expires_at='' OR op.lease_expires_at<=?)))
		ORDER BY op.created_at,op.id
		LIMIT 1`, "2026-01-01T00:00:00Z")

	assertNoFullScan(t, plan)
}

// The per-instance admission guards (COUNT(*) ... WHERE database_instance_id=?
// AND status IN (...)) run on every enqueue and full-scanned before 0027.
func TestDatabaseOperationInstanceGuardUsesAnIndex(t *testing.T) {
	t.Parallel()
	db, ctx := openFullyMigratedDB(t, "guard-index.db")

	plan := explainQueryPlan(t, db, ctx, `
		SELECT COUNT(*) FROM database_operations
		WHERE database_instance_id=? AND status IN ('queued','running')`, "instance-1")

	assertNoFullScan(t, plan)
}
