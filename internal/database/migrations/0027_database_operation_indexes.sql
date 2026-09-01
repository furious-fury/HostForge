-- Claim and enqueue-guard indexes for database_operations (ADR-0002 §4.2).
--
-- 0021 shipped only idx_database_operations_service(service_id, created_at DESC),
-- which serves the UI list but neither the 2-second claim poll nor the
-- per-instance admission guards. database_gateway_operations already has its
-- queue index (0022); this brings the two subsystems to parity.
--
-- The claim's status predicate is an OR across 'queued' and expired-lease
-- 'running', which SQLite satisfies as MULTI-INDEX OR: two index searches
-- instead of a full table scan. It still builds a temp b-tree for the
-- ORDER BY, because a multi-index OR cannot preserve index order — the win
-- here is the scan, not the sort. created_at and id are carried anyway so
-- the sort input comes from the index rather than the table.
-- lease_expires_at is deliberately excluded: it only filters the small
-- 'running' arm, and a wider index costs more on every write.
CREATE INDEX idx_database_operations_queue
    ON database_operations(status, created_at, id);

-- Serves the per-instance COUNT(*) admission guards, which full-scan today on
-- every enqueue.
CREATE INDEX idx_database_operations_instance
    ON database_operations(database_instance_id, status);
