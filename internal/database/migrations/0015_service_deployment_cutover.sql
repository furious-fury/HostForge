-- Make service/environment ownership authoritative for deployments while legacy
-- readers are removed in the same release train.
CREATE TABLE deployments_v2 (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL DEFAULT '',
    service_id TEXT,
    environment_id TEXT,
    status TEXT NOT NULL CHECK(status IN ('QUEUED', 'BUILDING', 'SUCCESS', 'FAILED', 'CANCELLED')),
    commit_hash TEXT NOT NULL DEFAULT '',
    logs_path TEXT NOT NULL DEFAULT '',
    image_ref TEXT NOT NULL DEFAULT '',
    worktree TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    builder_kind TEXT NOT NULL DEFAULT '',
    stack_kind TEXT NOT NULL DEFAULT '',
    stack_label TEXT NOT NULL DEFAULT '',
    trigger_kind TEXT NOT NULL DEFAULT 'manual',
    actor TEXT NOT NULL DEFAULT '',
    cancelled_at TEXT NOT NULL DEFAULT '',
    rollback_of TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(service_id) REFERENCES services(id) ON DELETE CASCADE,
    FOREIGN KEY(environment_id) REFERENCES environments(id) ON DELETE CASCADE
);

INSERT INTO deployments_v2(
    id, project_id, service_id, environment_id, status, commit_hash, logs_path,
    image_ref, worktree, error_message, builder_kind, stack_kind, stack_label,
    trigger_kind, actor, cancelled_at, rollback_of, created_at, updated_at
)
SELECT id, project_id, service_id, environment_id, status, commit_hash, logs_path,
       image_ref, worktree, error_message, builder_kind, stack_kind, stack_label,
       trigger_kind, actor, cancelled_at, rollback_of, created_at, updated_at
FROM deployments;

CREATE TABLE containers_v2 (
    id TEXT PRIMARY KEY,
    deployment_id TEXT NOT NULL,
    docker_container_id TEXT NOT NULL,
    internal_port INTEGER NOT NULL,
    host_port INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'RUNNING',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(deployment_id) REFERENCES deployments_v2(id) ON DELETE CASCADE
);
INSERT INTO containers_v2 SELECT id, deployment_id, docker_container_id, internal_port, host_port, status, created_at, updated_at FROM containers;

DROP TABLE containers;
DROP TABLE deployments;
ALTER TABLE deployments_v2 RENAME TO deployments;
ALTER TABLE containers_v2 RENAME TO containers;

CREATE INDEX idx_deployments_project_id ON deployments(project_id);
CREATE INDEX idx_deployments_service_environment ON deployments(service_id, environment_id, created_at DESC);
CREATE INDEX idx_deployments_status ON deployments(status);
CREATE INDEX idx_containers_deployment_id ON containers(deployment_id);

UPDATE service_environments
SET active_deployment_id = COALESCE((
    SELECT d.id
    FROM deployments d
    WHERE d.service_id = service_environments.service_id
      AND d.environment_id = service_environments.environment_id
      AND d.status = 'SUCCESS'
    ORDER BY d.created_at DESC
    LIMIT 1
), '');

