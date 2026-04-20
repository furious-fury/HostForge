-- Nixpacks plan summary (captured at deploy time for UI badges).
ALTER TABLE deployments ADD COLUMN stack_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN stack_label TEXT NOT NULL DEFAULT '';
