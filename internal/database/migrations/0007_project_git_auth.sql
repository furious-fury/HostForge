CREATE TABLE IF NOT EXISTS project_git_auth (
    project_id TEXT PRIMARY KEY,
    provider TEXT NOT NULL DEFAULT 'github',
    token_ct BLOB NOT NULL,
    token_last4 TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
);
