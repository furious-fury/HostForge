-- Singleton row proving HOSTFORGE_ENV_ENCRYPTION_KEY at boot matches the key
-- that sealed every other secret already in this database (env vars,
-- database credentials, the GitHub App private key). Seeded on first boot;
-- checked, never rewritten, on every boot after. See internal/crypto/envcrypt.
CREATE TABLE IF NOT EXISTS encryption_canary (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    canary_ct BLOB NOT NULL,
    created_at TEXT NOT NULL
);
