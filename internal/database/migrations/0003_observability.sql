-- Local observability samples for UI (bounded retention; no external sink).

CREATE TABLE IF NOT EXISTS deploy_steps (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	deployment_id TEXT NOT NULL,
	project_id TEXT NOT NULL DEFAULT '',
	request_id TEXT NOT NULL DEFAULT '',
	step TEXT NOT NULL,
	status TEXT NOT NULL,
	duration_ms INTEGER,
	error_code TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	ended_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_deploy_steps_deployment_id ON deploy_steps(deployment_id);
CREATE INDEX IF NOT EXISTS idx_deploy_steps_project_ended ON deploy_steps(project_id, ended_at DESC);
CREATE INDEX IF NOT EXISTS idx_deploy_steps_ended ON deploy_steps(ended_at DESC);

CREATE TABLE IF NOT EXISTS http_requests (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	request_id TEXT NOT NULL,
	method TEXT NOT NULL,
	path TEXT NOT NULL,
	status INTEGER NOT NULL,
	duration_ms INTEGER NOT NULL,
	started_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_http_requests_started ON http_requests(started_at DESC);
