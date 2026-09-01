-- The generic operations queue (ADR-0002 §4).
--
-- Every durable background job moves onto this table over the phases that
-- follow; this migration adds it and backfills the existing
-- database_operations rows. database_operations is NOT replaced: three
-- foreign keys point at it (database_backups.operation_id,
-- database_restore_jobs.operation_id and database_upgrade_jobs.operation_id,
-- the last two as PRIMARY KEY), and a view cannot satisfy a foreign key.
--
-- Instead the two share a primary key: operations.id == database_operations.id,
-- one identity, two rows. operations is authoritative for queueing — status,
-- lease, attempt, priority, scheduling, cancellation — and database_operations
-- is kept in step as a projection, so it stays the FK anchor and the read
-- model every existing API and screen already uses. That is what lets this
-- land without changing a single response shape or status string.
--
-- Sentinels follow the rest of the schema: TEXT NOT NULL DEFAULT '' rather
-- than NULL, so no read path has to distinguish the two.
CREATE TABLE operations (
    id TEXT PRIMARY KEY,

    -- kind selects the handler. Namespaced by subsystem ('db_provision',
    -- 'db_backup', ...) so deploys and future subsystems cannot collide.
    kind TEXT NOT NULL,

    -- lock_key serialises execution: the claim query will not start an
    -- operation while another with the same key is running. Replaces the
    -- enqueue-time COUNT(*) guards, which four of seven insert sites
    -- bypassed. 'dbi:<instance id>' for instance work, 'dbsvc:<service id>'
    -- for service-scoped work.
    lock_key TEXT NOT NULL,

    status TEXT NOT NULL DEFAULT 'queued'
        CHECK(status IN ('queued', 'running', 'success', 'failed', 'cancelled')),
    progress_step TEXT NOT NULL DEFAULT '',
    progress_percent INTEGER NOT NULL DEFAULT 0 CHECK(progress_percent BETWEEN 0 AND 100),

    -- Higher runs first (§20.2). 100 is the default band; interactive work
    -- can be raised above it later without a schema change.
    priority INTEGER NOT NULL DEFAULT 100,

    -- available_at defers an operation without consuming an attempt: a
    -- handler that is not ready yet is rescheduled rather than failed.
    available_at TEXT NOT NULL DEFAULT '',

    attempt INTEGER NOT NULL DEFAULT 0 CHECK(attempt >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 5 CHECK(max_attempts >= 1),

    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at TEXT NOT NULL DEFAULT '',

    -- Set to request cooperative cancellation. The worker observes it on its
    -- lease-renewal tick and cancels the operation context, so cancellation
    -- lands at a step boundary rather than mid-write.
    cancel_requested_at TEXT NOT NULL DEFAULT '',

    -- Scope, denormalised for filtering and for future API surfaces. Not
    -- foreign keys: an operation outlives the rows it refers to, and its
    -- history should survive their deletion.
    application_id TEXT NOT NULL DEFAULT '',
    service_id TEXT NOT NULL DEFAULT '',
    environment_id TEXT NOT NULL DEFAULT '',

    actor TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL DEFAULT '',
    completed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- The claim: filter on status, order by priority then age. Carrying
-- available_at makes the readiness filter index-resident too.
CREATE INDEX idx_operations_claim
    ON operations(status, priority DESC, created_at, id, available_at);

-- The claim's serialisation check, which asks whether any running operation
-- holds a given lock_key.
CREATE INDEX idx_operations_lock
    ON operations(lock_key, status);

CREATE INDEX idx_operations_service
    ON operations(service_id, created_at DESC);

-- Backfill. Two cases here only ever appear on a database that has been in
-- use, never on a fresh one, so they are the migration's real risk:
--
--   1. Terminal 'delete' audit rows written by FinalizeDatabaseServiceDeletion
--      have database_instance_id NULL. Keying lock_key off the instance alone
--      would produce NULL and violate NOT NULL, so those fall back to the
--      service scope.
--   2. environment_id is only reachable through the instance, so rows without
--      one resolve to ''.
--
-- Runs inside ApplyMigrations' transaction, at boot, before any worker
-- starts, so there is no concurrent writer and in-flight rows carry their
-- status, attempt_count and lease across intact.
INSERT INTO operations (
    id, kind, lock_key, status, progress_step, progress_percent,
    priority, available_at, attempt, max_attempts,
    lease_owner, lease_expires_at, cancel_requested_at,
    application_id, service_id, environment_id,
    actor, error_code, error_message,
    started_at, completed_at, created_at, updated_at
)
SELECT
    op.id,
    'db_' || op.operation_type,
    CASE
        WHEN op.database_instance_id IS NOT NULL AND op.database_instance_id <> ''
            THEN 'dbi:' || op.database_instance_id
        ELSE 'dbsvc:' || op.service_id
    END,
    op.status, op.progress_step, op.progress_percent,
    100, '', op.attempt_count, 5,
    op.lease_owner, op.lease_expires_at, '',
    COALESCE(svc.application_id, ''), op.service_id, COALESCE(inst.environment_id, ''),
    op.actor, op.error_code, op.error_message,
    op.started_at, op.completed_at, op.created_at, op.updated_at
FROM database_operations op
LEFT JOIN services svc ON svc.id = op.service_id
LEFT JOIN database_instances inst ON inst.id = op.database_instance_id;
