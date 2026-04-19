CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	repo_url TEXT NOT NULL,
	branch TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE(repo_url, branch)
);

CREATE TABLE IF NOT EXISTS deployments (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	status TEXT NOT NULL CHECK(status IN ('QUEUED', 'BUILDING', 'SUCCESS', 'FAILED')),
	commit_hash TEXT NOT NULL DEFAULT '',
	logs_path TEXT NOT NULL DEFAULT '',
	image_ref TEXT NOT NULL DEFAULT '',
	worktree TEXT NOT NULL DEFAULT '',
	error_message TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS domains (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	domain_name TEXT NOT NULL UNIQUE,
	ssl_status TEXT NOT NULL DEFAULT 'PENDING',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS containers (
	id TEXT PRIMARY KEY,
	deployment_id TEXT NOT NULL,
	docker_container_id TEXT NOT NULL,
	internal_port INTEGER NOT NULL,
	host_port INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT 'RUNNING',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY(deployment_id) REFERENCES deployments(id)
);

CREATE INDEX IF NOT EXISTS idx_deployments_project_id ON deployments(project_id);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
CREATE INDEX IF NOT EXISTS idx_containers_deployment_id ON containers(deployment_id);
