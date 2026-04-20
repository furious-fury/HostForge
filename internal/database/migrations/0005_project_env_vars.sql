-- Encrypted per-project environment variables (runtime injection into containers).

CREATE TABLE IF NOT EXISTS project_env_vars (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	key TEXT NOT NULL,
	value_ct BLOB NOT NULL,
	value_last4 TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
	UNIQUE(project_id, key)
);

CREATE INDEX IF NOT EXISTS idx_project_env_vars_project ON project_env_vars(project_id);
