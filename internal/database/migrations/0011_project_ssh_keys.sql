-- Per-project SSH deploy key (ed25519). The public key is shown once to the operator
-- so they can register it as a GitHub "Deploy key" on the repo. The private key is
-- sealed via envcrypt.Sealer (HOSTFORGE_ENV_ENCRYPTION_KEY).
CREATE TABLE IF NOT EXISTS project_ssh_keys (
    project_id TEXT PRIMARY KEY,
    public_key TEXT NOT NULL,
    private_key_ct BLOB NOT NULL,
    fingerprint TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
);
