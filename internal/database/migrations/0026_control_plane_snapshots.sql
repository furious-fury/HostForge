-- Tracks VACUUM INTO snapshots of hostforge.db taken by the scheduled
-- control-plane snapshot loop (ADR-0002 §17.2). Not used by the separate
-- pre-migration snapshot in internal/database/sqlite.go, which is a plain
-- untracked file next to hostforge.db and runs before this table (or any
-- table) necessarily exists.
--
-- No 'pending' status: the loop runs synchronously (no operations queue
-- exists yet for this — Phase 1 of the ADR), so a row is only ever inserted
-- already 'running', then updated to 'success' or 'failed'.
CREATE TABLE control_plane_snapshots (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL CHECK(status IN ('running', 'success', 'failed')),
    snapshot_path TEXT NOT NULL DEFAULT '',
    remote_key TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_control_plane_snapshots_created ON control_plane_snapshots(created_at DESC);
