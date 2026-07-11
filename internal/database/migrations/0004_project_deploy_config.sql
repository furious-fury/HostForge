-- Per-project Nixpacks / Bun deploy overrides (HostForge-managed defaults + custom commands).

ALTER TABLE projects ADD COLUMN deploy_runtime TEXT NOT NULL DEFAULT 'auto'
  CHECK (deploy_runtime IN ('auto', 'bun'));
ALTER TABLE projects ADD COLUMN deploy_install_cmd TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN deploy_build_cmd TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN deploy_start_cmd TEXT NOT NULL DEFAULT '';
