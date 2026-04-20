-- Track which git credential source a project uses.
-- git_source values: 'url' (default, public or legacy PAT-only), 'github_app', 'ssh'.
-- github_installation_id is used when git_source='github_app' to pick installation token.
ALTER TABLE projects ADD COLUMN git_source TEXT NOT NULL DEFAULT 'url';
ALTER TABLE projects ADD COLUMN github_installation_id INTEGER NOT NULL DEFAULT 0;
