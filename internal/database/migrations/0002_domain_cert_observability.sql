-- Optional Caddy certificate lifecycle hints (detailed backlog §2.1).
-- last_cert_message: operator-facing summary from on-disk cert scan (not ACME state machine).
-- cert_checked_at: RFC3339 UTC when HostForge last ran the poll for this row.

ALTER TABLE domains ADD COLUMN last_cert_message TEXT NOT NULL DEFAULT '';
ALTER TABLE domains ADD COLUMN cert_checked_at TEXT NOT NULL DEFAULT '';
