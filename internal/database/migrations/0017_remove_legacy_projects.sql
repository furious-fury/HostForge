-- Finalize the application/service model. All legacy projects were materialized as
-- applications and production services by migration 0014.
CREATE TABLE deployments_final (
	id TEXT PRIMARY KEY,
	service_id TEXT NOT NULL,
	environment_id TEXT NOT NULL,
	status TEXT NOT NULL CHECK(status IN ('QUEUED','BUILDING','SUCCESS','FAILED','CANCELLED')),
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
INSERT INTO deployments_final
SELECT id,service_id,environment_id,status,commit_hash,logs_path,image_ref,worktree,
       error_message,builder_kind,stack_kind,stack_label,trigger_kind,actor,
       cancelled_at,rollback_of,created_at,updated_at
FROM deployments
WHERE service_id IS NOT NULL AND service_id<>'' AND environment_id IS NOT NULL AND environment_id<>'';

CREATE TABLE containers_final (
	id TEXT PRIMARY KEY,
	deployment_id TEXT NOT NULL,
	docker_container_id TEXT NOT NULL,
	internal_port INTEGER NOT NULL,
	host_port INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT 'RUNNING',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY(deployment_id) REFERENCES deployments_final(id) ON DELETE CASCADE
);
INSERT INTO containers_final
SELECT c.id,c.deployment_id,c.docker_container_id,c.internal_port,c.host_port,c.status,c.created_at,c.updated_at
FROM containers c JOIN deployments_final d ON d.id=c.deployment_id;

DROP TABLE containers;
DROP TABLE deployments;
ALTER TABLE deployments_final RENAME TO deployments;
ALTER TABLE containers_final RENAME TO containers;
CREATE INDEX idx_deployments_service_environment ON deployments(service_id,environment_id,created_at DESC);
CREATE INDEX idx_deployments_status ON deployments(status);
CREATE INDEX idx_containers_deployment_id ON containers(deployment_id);

CREATE TABLE domains_final (
	id TEXT PRIMARY KEY,
	application_id TEXT NOT NULL,
	environment_id TEXT NOT NULL,
	service_id TEXT NOT NULL,
	domain_name TEXT NOT NULL UNIQUE,
	ssl_status TEXT NOT NULL DEFAULT 'PENDING',
	last_cert_message TEXT NOT NULL DEFAULT '',
	cert_checked_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY(application_id) REFERENCES applications(id) ON DELETE CASCADE,
	FOREIGN KEY(environment_id) REFERENCES environments(id) ON DELETE CASCADE,
	FOREIGN KEY(service_id) REFERENCES services(id) ON DELETE CASCADE
);
INSERT INTO domains_final
SELECT id,application_id,environment_id,service_id,domain_name,ssl_status,
       last_cert_message,cert_checked_at,created_at,updated_at
FROM domains
WHERE application_id<>'' AND environment_id<>'' AND service_id<>'';
DROP TABLE domains;
ALTER TABLE domains_final RENAME TO domains;
CREATE INDEX idx_domains_application_environment ON domains(application_id,environment_id,domain_name);
CREATE INDEX idx_domains_service_environment ON domains(service_id,environment_id,domain_name);

CREATE TABLE deploy_steps_final (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	deployment_id TEXT NOT NULL DEFAULT '',
	service_id TEXT NOT NULL DEFAULT '',
	environment_id TEXT NOT NULL DEFAULT '',
	request_id TEXT NOT NULL DEFAULT '',
	step TEXT NOT NULL,
	status TEXT NOT NULL,
	duration_ms INTEGER,
	error_code TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	ended_at TEXT NOT NULL
);
INSERT INTO deploy_steps_final(id,deployment_id,service_id,environment_id,request_id,step,status,duration_ms,error_code,started_at,ended_at)
SELECT ds.id,ds.deployment_id,COALESCE(d.service_id,''),COALESCE(d.environment_id,''),
       ds.request_id,ds.step,ds.status,ds.duration_ms,ds.error_code,ds.started_at,ds.ended_at
FROM deploy_steps ds LEFT JOIN deployments d ON d.id=ds.deployment_id;
DROP TABLE deploy_steps;
ALTER TABLE deploy_steps_final RENAME TO deploy_steps;
CREATE INDEX idx_deploy_steps_deployment_id ON deploy_steps(deployment_id);
CREATE INDEX idx_deploy_steps_service_environment_ended ON deploy_steps(service_id,environment_id,ended_at DESC);
CREATE INDEX idx_deploy_steps_ended ON deploy_steps(ended_at DESC);

DROP TABLE project_env_vars;
DROP TABLE project_git_auth;
DROP TABLE project_ssh_keys;
DROP TABLE projects;
