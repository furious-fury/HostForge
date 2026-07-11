-- Persistent state for the one-time public-IP HTTPS onboarding flow.
CREATE TABLE IF NOT EXISTS onboarding_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    github_app_complete INTEGER NOT NULL DEFAULT 0,
    platform_domain TEXT NOT NULL DEFAULT '',
    permanent_ingress_complete INTEGER NOT NULL DEFAULT 0,
    bootstrap_complete INTEGER NOT NULL DEFAULT 0,
    completed_at TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);

INSERT OR IGNORE INTO onboarding_state(id, updated_at) VALUES (1, datetime('now'));
