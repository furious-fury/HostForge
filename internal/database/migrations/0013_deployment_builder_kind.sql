-- Builder selected for this deployment: railpack or dockerfile.
ALTER TABLE deployments ADD COLUMN builder_kind TEXT NOT NULL DEFAULT '';
