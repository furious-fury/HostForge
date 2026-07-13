-- Application -> environment -> service model. Legacy project columns remain
-- during the cutover so existing installations can migrate without rewriting IDs.
CREATE TABLE applications (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE environments (
    id TEXT PRIMARY KEY,
    application_id TEXT NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('production', 'staging')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(application_id) REFERENCES applications(id) ON DELETE CASCADE,
    UNIQUE(application_id, slug)
);

CREATE TABLE services (
    id TEXT PRIMARY KEY,
    application_id TEXT NOT NULL,
    name TEXT NOT NULL,
    repo_url TEXT NOT NULL,
    github_installation_id INTEGER NOT NULL DEFAULT 0,
    root_directory TEXT NOT NULL DEFAULT '',
    deploy_runtime TEXT NOT NULL DEFAULT 'auto',
    deploy_install_cmd TEXT NOT NULL DEFAULT '',
    deploy_build_cmd TEXT NOT NULL DEFAULT '',
    deploy_start_cmd TEXT NOT NULL DEFAULT '',
    internal_port INTEGER NOT NULL DEFAULT 3000,
    health_check_path TEXT NOT NULL DEFAULT '/health',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(application_id) REFERENCES applications(id) ON DELETE CASCADE,
    UNIQUE(application_id, name)
);

CREATE TABLE service_environments (
    service_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT '',
    auto_deploy INTEGER NOT NULL DEFAULT 0,
    active_deployment_id TEXT NOT NULL DEFAULT '',
    desired_state TEXT NOT NULL DEFAULT 'running' CHECK(desired_state IN ('running', 'stopped')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY(service_id, environment_id),
    FOREIGN KEY(service_id) REFERENCES services(id) ON DELETE CASCADE,
    FOREIGN KEY(environment_id) REFERENCES environments(id) ON DELETE CASCADE
);

ALTER TABLE deployments ADD COLUMN service_id TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN environment_id TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN trigger_kind TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE deployments ADD COLUMN actor TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN cancelled_at TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN rollback_of TEXT NOT NULL DEFAULT '';

ALTER TABLE domains ADD COLUMN application_id TEXT NOT NULL DEFAULT '';
ALTER TABLE domains ADD COLUMN environment_id TEXT NOT NULL DEFAULT '';
ALTER TABLE domains ADD COLUMN service_id TEXT NOT NULL DEFAULT '';

CREATE TABLE environment_variables (
    id TEXT PRIMARY KEY,
    application_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    service_id TEXT,
    key TEXT NOT NULL,
    value_ct BLOB NOT NULL,
    value_last4 TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(application_id) REFERENCES applications(id) ON DELETE CASCADE,
    FOREIGN KEY(environment_id) REFERENCES environments(id) ON DELETE CASCADE,
    FOREIGN KEY(service_id) REFERENCES services(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_environment_variables_scope
ON environment_variables(environment_id, IFNULL(service_id, ''), key);

CREATE TABLE platform_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    application_id TEXT NOT NULL DEFAULT '',
    service_id TEXT NOT NULL DEFAULT '',
    environment_id TEXT NOT NULL DEFAULT '',
    deployment_id TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE TABLE service_metric_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    cpu_percent REAL NOT NULL,
    memory_bytes INTEGER NOT NULL,
    network_rx_bytes INTEGER NOT NULL DEFAULT 0,
    network_tx_bytes INTEGER NOT NULL DEFAULT 0,
    sampled_at TEXT NOT NULL
);

-- Preserve legacy IDs where possible: project ID becomes both application and service ID.
INSERT INTO applications(id, name, created_at, updated_at)
SELECT id, name, created_at, updated_at FROM projects;
INSERT INTO environments(id, application_id, name, slug, kind, created_at, updated_at)
SELECT id || '_production', id, 'Production', 'production', 'production', created_at, updated_at FROM projects;
INSERT INTO environments(id, application_id, name, slug, kind, created_at, updated_at)
SELECT id || '_staging', id, 'Staging', 'staging', 'staging', created_at, updated_at FROM projects;
INSERT INTO services(id, application_id, name, repo_url, github_installation_id, deploy_runtime,
 deploy_install_cmd, deploy_build_cmd, deploy_start_cmd, created_at, updated_at)
SELECT id, id, name, repo_url, github_installation_id, deploy_runtime,
 deploy_install_cmd, deploy_build_cmd, deploy_start_cmd, created_at, updated_at FROM projects;
INSERT INTO service_environments(service_id, environment_id, branch, auto_deploy, created_at, updated_at)
SELECT id, id || '_production', branch, 0, created_at, updated_at FROM projects;
INSERT INTO service_environments(service_id, environment_id, branch, auto_deploy, created_at, updated_at)
SELECT id, id || '_staging', '', 0, created_at, updated_at FROM projects;
UPDATE deployments SET service_id = project_id, environment_id = project_id || '_production';
UPDATE domains SET application_id = project_id, environment_id = project_id || '_production', service_id = project_id;
INSERT INTO environment_variables(id, application_id, environment_id, service_id, key, value_ct, value_last4, created_at, updated_at)
SELECT id, project_id, project_id || '_production', NULL, key, value_ct, value_last4, created_at, updated_at
FROM project_env_vars;

CREATE INDEX idx_environments_application ON environments(application_id);
CREATE INDEX idx_services_application ON services(application_id);
CREATE INDEX idx_deployments_service_environment ON deployments(service_id, environment_id, created_at DESC);
CREATE INDEX idx_events_created ON platform_events(created_at DESC);
CREATE INDEX idx_service_metrics_scope ON service_metric_samples(service_id, environment_id, sampled_at DESC);
