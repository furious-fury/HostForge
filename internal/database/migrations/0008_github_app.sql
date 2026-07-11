-- Singleton GitHub App registration.
-- There is at most one App configured per HostForge server (enforced via primary key).
-- Sensitive columns (client_secret, private_key, webhook_secret) are sealed with
-- HOSTFORGE_ENV_ENCRYPTION_KEY via internal/crypto/envcrypt before insert.
CREATE TABLE IF NOT EXISTS github_app (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    app_id INTEGER NOT NULL,
    slug TEXT NOT NULL DEFAULT '',
    html_url TEXT NOT NULL DEFAULT '',
    client_id TEXT NOT NULL DEFAULT '',
    client_secret_ct BLOB,
    private_key_ct BLOB NOT NULL,
    webhook_secret_ct BLOB,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
