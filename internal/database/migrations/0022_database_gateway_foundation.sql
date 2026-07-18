-- Multi-engine database gateway control-plane foundation. PostgreSQL is the
-- only data-plane adapter registered in v1. SQLite remains the source of truth;
-- gateway containers, link networks, roles, and rendered files are derived.
CREATE TABLE database_gateway_endpoints (
    engine TEXT PRIMARY KEY
        CHECK(engine IN ('postgresql', 'mysql', 'mariadb', 'mongodb', 'redis', 'valkey')),
    hostname TEXT NOT NULL,
    port INTEGER NOT NULL CHECK(port BETWEEN 1 AND 65535),
    image_ref TEXT NOT NULL DEFAULT '',
    image_version TEXT NOT NULL DEFAULT '',
    container_name TEXT NOT NULL,
    docker_container_id TEXT NOT NULL DEFAULT '',
    ingress_network_name TEXT NOT NULL,
    desired_status TEXT NOT NULL DEFAULT 'absent'
        CHECK(desired_status IN ('absent', 'active', 'deleting')),
    observed_status TEXT NOT NULL DEFAULT 'absent'
        CHECK(observed_status IN ('absent', 'provisioning', 'active', 'degraded', 'failed', 'deleting')),
    certificate_fingerprint TEXT NOT NULL DEFAULT '',
    certificate_expires_at TEXT NOT NULL DEFAULT '',
    certificate_synced_at TEXT NOT NULL DEFAULT '',
    desired_config_generation INTEGER NOT NULL DEFAULT 0 CHECK(desired_config_generation >= 0),
    rendered_config_generation INTEGER NOT NULL DEFAULT 0 CHECK(rendered_config_generation >= 0),
    applied_config_generation INTEGER NOT NULL DEFAULT 0 CHECK(applied_config_generation >= 0),
    last_error_code TEXT NOT NULL DEFAULT '',
    last_error_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(hostname, port),
    UNIQUE(container_name),
    UNIQUE(ingress_network_name)
);

CREATE TABLE database_gateway_routes (
    id TEXT PRIMARY KEY,
    engine TEXT NOT NULL
        CHECK(engine IN ('postgresql', 'mysql', 'mariadb', 'mongodb', 'redis', 'valkey')),
    database_instance_id TEXT NOT NULL UNIQUE,
    route_alias TEXT NOT NULL UNIQUE,
    backend_alias TEXT NOT NULL UNIQUE,
    link_network_name TEXT NOT NULL UNIQUE,
    desired_status TEXT NOT NULL DEFAULT 'active'
        CHECK(desired_status IN ('active', 'disabled', 'deleted')),
    observed_status TEXT NOT NULL DEFAULT 'pending'
        CHECK(observed_status IN ('pending', 'active', 'disabled', 'failed', 'deleted')),
    route_backend_limit INTEGER NOT NULL CHECK(route_backend_limit BETWEEN 1 AND 50),
    credential_backend_limit INTEGER NOT NULL CHECK(credential_backend_limit BETWEEN 1 AND 50),
    last_error_code TEXT NOT NULL DEFAULT '',
    last_error_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(engine) REFERENCES database_gateway_endpoints(engine) ON DELETE RESTRICT,
    FOREIGN KEY(database_instance_id) REFERENCES database_instances(id) ON DELETE CASCADE
);

CREATE TABLE database_external_connections (
    id TEXT PRIMARY KEY,
    route_id TEXT NOT NULL,
    name TEXT NOT NULL,
    permission_profile TEXT NOT NULL
        CHECK(permission_profile IN ('read_only', 'read_write', 'migration')),
    expires_at TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK(status IN ('pending', 'active', 'disabled', 'expired', 'rotating', 'revoking', 'revoked', 'failed')),
    current_generation INTEGER NOT NULL DEFAULT 0 CHECK(current_generation >= 0),
    client_connection_limit INTEGER NOT NULL DEFAULT 20
        CHECK(client_connection_limit BETWEEN 1 AND 20),
    last_used_at TEXT NOT NULL DEFAULT '',
    last_error_code TEXT NOT NULL DEFAULT '',
    last_error_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(route_id) REFERENCES database_gateway_routes(id) ON DELETE CASCADE,
    UNIQUE(route_id, name)
);

CREATE TABLE database_external_credentials (
    id TEXT PRIMARY KEY,
    connection_id TEXT NOT NULL,
    role_name TEXT NOT NULL UNIQUE,
    password_ct BLOB NOT NULL DEFAULT X'',
    scram_verifier_ct BLOB NOT NULL DEFAULT X'',
    generation INTEGER NOT NULL CHECK(generation > 0),
    state TEXT NOT NULL DEFAULT 'active'
        CHECK(state IN ('active', 'grace', 'revoked')),
    grace_deadline TEXT NOT NULL DEFAULT '',
    last_used_at TEXT NOT NULL DEFAULT '',
    revoked_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(connection_id) REFERENCES database_external_connections(id) ON DELETE CASCADE,
    UNIQUE(connection_id, generation)
);

CREATE TABLE database_external_connection_cidrs (
    connection_id TEXT NOT NULL,
    cidr TEXT NOT NULL,
    address_family INTEGER NOT NULL CHECK(address_family IN (4, 6)),
    created_at TEXT NOT NULL,
    PRIMARY KEY(connection_id, cidr),
    FOREIGN KEY(connection_id) REFERENCES database_external_connections(id) ON DELETE CASCADE
);

CREATE TABLE database_gateway_operations (
    id TEXT PRIMARY KEY,
    engine TEXT NOT NULL
        CHECK(engine IN ('postgresql', 'mysql', 'mariadb', 'mongodb', 'redis', 'valkey')),
    route_id TEXT,
    connection_id TEXT,
    credential_id TEXT,
    operation_type TEXT NOT NULL CHECK(operation_type IN (
        'provision_gateway', 'teardown_gateway', 'create_connection', 'update_connection',
        'disable_connection', 'enable_connection', 'rotate_connection', 'retire_credential',
        'revoke_connection', 'expire_connection', 'reconcile_route'
    )),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK(status IN ('queued', 'running', 'success', 'failed', 'cancelled')),
    progress_step TEXT NOT NULL DEFAULT '',
    progress_percent INTEGER NOT NULL DEFAULT 0 CHECK(progress_percent BETWEEN 0 AND 100),
    requested_grace_period_hours INTEGER NOT NULL DEFAULT 24
        CHECK(requested_grace_period_hours BETWEEN 0 AND 168),
    actor TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL DEFAULT '',
    completed_at TEXT NOT NULL DEFAULT '',
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at TEXT NOT NULL DEFAULT '',
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK(attempt_count >= 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(route_id) REFERENCES database_gateway_routes(id) ON DELETE SET NULL,
    FOREIGN KEY(connection_id) REFERENCES database_external_connections(id) ON DELETE SET NULL,
    FOREIGN KEY(credential_id) REFERENCES database_external_credentials(id) ON DELETE SET NULL
);

CREATE INDEX idx_database_gateway_routes_engine_status
    ON database_gateway_routes(engine, desired_status, observed_status);
CREATE INDEX idx_database_external_connections_route_status
    ON database_external_connections(route_id, status, created_at DESC);
CREATE INDEX idx_database_external_connections_expiry
    ON database_external_connections(expires_at, status) WHERE expires_at <> '';
CREATE INDEX idx_database_external_credentials_connection_state
    ON database_external_credentials(connection_id, state, generation DESC);
CREATE INDEX idx_database_external_credentials_grace
    ON database_external_credentials(grace_deadline, state) WHERE grace_deadline <> '';
CREATE INDEX idx_database_gateway_operations_queue
    ON database_gateway_operations(status, created_at);
CREATE INDEX idx_database_gateway_operations_connection
    ON database_gateway_operations(connection_id, created_at DESC);
