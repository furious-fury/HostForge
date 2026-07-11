-- GitHub App installations (one row per account the operator installed the App on).
-- Populated via the App API (internal/github/app.ListInstallations) and installation webhooks.
CREATE TABLE IF NOT EXISTS github_app_installations (
    installation_id INTEGER PRIMARY KEY,
    account_login TEXT NOT NULL DEFAULT '',
    account_type TEXT NOT NULL DEFAULT '',
    target_type TEXT NOT NULL DEFAULT '',
    repo_selection TEXT NOT NULL DEFAULT '',
    suspended_at TEXT NOT NULL DEFAULT '',
    last_synced_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_github_app_installations_account ON github_app_installations(account_login);
