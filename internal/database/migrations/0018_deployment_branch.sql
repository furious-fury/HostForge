-- Persist the source branch on each deployment so historical filters and audit
-- output do not change when a service-environment binding is reconfigured.
ALTER TABLE deployments ADD COLUMN branch TEXT NOT NULL DEFAULT '';

UPDATE deployments
SET branch = COALESCE((
  SELECT se.branch FROM service_environments se
  WHERE se.service_id = deployments.service_id
    AND se.environment_id = deployments.environment_id
), '');

CREATE INDEX idx_deployments_branch_created ON deployments(branch, created_at DESC);
