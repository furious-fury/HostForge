-- Persistent database-service foundation. Database instances intentionally use
-- their own environment lifecycle instead of application deployment bindings.
ALTER TABLE services
ADD COLUMN service_type TEXT NOT NULL DEFAULT 'application'
CHECK(service_type IN ('application', 'database'));

CREATE TABLE database_services (
    service_id TEXT PRIMARY KEY,
    engine TEXT NOT NULL CHECK(engine IN ('postgresql', 'mysql', 'mariadb', 'mongodb', 'redis', 'valkey')),
    default_version TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(service_id) REFERENCES services(id) ON DELETE CASCADE
);

CREATE TABLE database_instances (
    id TEXT PRIMARY KEY,
    service_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    engine_version TEXT NOT NULL,
    image_ref TEXT NOT NULL,
    docker_container_id TEXT NOT NULL DEFAULT '',
    network_alias TEXT NOT NULL,
    internal_port INTEGER NOT NULL CHECK(internal_port BETWEEN 1 AND 65535),
    volume_name TEXT NOT NULL,
    resource_preset TEXT NOT NULL CHECK(resource_preset IN ('development', 'standard', 'performance', 'custom')),
    cpu_limit_millis INTEGER NOT NULL CHECK(cpu_limit_millis > 0),
    memory_limit_bytes INTEGER NOT NULL CHECK(memory_limit_bytes > 0),
    desired_state TEXT NOT NULL DEFAULT 'running' CHECK(desired_state IN ('running', 'stopped', 'deleted')),
    status TEXT NOT NULL DEFAULT 'provisioning'
        CHECK(status IN ('provisioning', 'starting', 'healthy', 'unhealthy', 'stopping', 'stopped', 'failed', 'deleted', 'purging')),
    health_message TEXT NOT NULL DEFAULT '',
    health_checked_at TEXT NOT NULL DEFAULT '',
    storage_used_bytes INTEGER NOT NULL DEFAULT 0 CHECK(storage_used_bytes >= 0),
    storage_checked_at TEXT NOT NULL DEFAULT '',
    deleted_at TEXT NOT NULL DEFAULT '',
    purge_after TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(service_id) REFERENCES database_services(service_id) ON DELETE CASCADE,
    FOREIGN KEY(environment_id) REFERENCES environments(id) ON DELETE CASCADE,
    UNIQUE(service_id, environment_id),
    UNIQUE(environment_id, network_alias),
    UNIQUE(volume_name)
);

CREATE TABLE database_credentials (
    database_instance_id TEXT PRIMARY KEY,
    database_name TEXT NOT NULL,
    username TEXT NOT NULL,
    password_ct BLOB NOT NULL,
    admin_password_ct BLOB NOT NULL DEFAULT X'',
    pending_password_ct BLOB NOT NULL DEFAULT X'',
    pending_created_at TEXT NOT NULL DEFAULT '',
    generation INTEGER NOT NULL DEFAULT 1 CHECK(generation > 0),
    rotated_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(database_instance_id) REFERENCES database_instances(id) ON DELETE CASCADE
);

CREATE TABLE database_bindings (
    id TEXT PRIMARY KEY,
    database_instance_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    consumer_service_id TEXT NOT NULL,
    variable_key TEXT NOT NULL,
    replace_existing INTEGER NOT NULL DEFAULT 0 CHECK(replace_existing IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(database_instance_id) REFERENCES database_instances(id) ON DELETE CASCADE,
    FOREIGN KEY(environment_id) REFERENCES environments(id) ON DELETE CASCADE,
    FOREIGN KEY(consumer_service_id) REFERENCES services(id) ON DELETE CASCADE,
    UNIQUE(database_instance_id, consumer_service_id, variable_key),
    UNIQUE(environment_id, consumer_service_id, variable_key)
);

CREATE TABLE backup_destinations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    provider TEXT NOT NULL CHECK(provider IN ('r2', 's3')),
    endpoint TEXT NOT NULL,
    region TEXT NOT NULL,
    bucket TEXT NOT NULL,
    object_prefix TEXT NOT NULL DEFAULT '',
    path_style INTEGER NOT NULL DEFAULT 0 CHECK(path_style IN (0, 1)),
    server_side_encryption TEXT NOT NULL DEFAULT '' CHECK(server_side_encryption IN ('', 'AES256', 'aws:kms')),
    sse_kms_key_id TEXT NOT NULL DEFAULT '',
    access_key_ct BLOB NOT NULL,
    secret_key_ct BLOB NOT NULL,
    last_test_status TEXT NOT NULL DEFAULT '',
    last_test_message TEXT NOT NULL DEFAULT '',
    last_tested_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE database_backup_policies (
    database_instance_id TEXT PRIMARY KEY,
    destination_id TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0, 1)),
    schedule TEXT NOT NULL DEFAULT '0 2 * * *',
    timezone TEXT NOT NULL DEFAULT 'UTC',
    retention_days INTEGER NOT NULL DEFAULT 30 CHECK(retention_days > 0),
    last_scheduled_at TEXT NOT NULL DEFAULT '',
    next_scheduled_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(database_instance_id) REFERENCES database_instances(id) ON DELETE CASCADE,
    FOREIGN KEY(destination_id) REFERENCES backup_destinations(id) ON DELETE RESTRICT
);

CREATE TABLE database_backups (
    id TEXT PRIMARY KEY,
    operation_id TEXT UNIQUE,
    database_instance_id TEXT,
    destination_id TEXT,
    status TEXT NOT NULL CHECK(status IN ('queued', 'running', 'success', 'failed', 'cancelled', 'deleting')),
    trigger_kind TEXT NOT NULL CHECK(trigger_kind IN ('manual', 'scheduled', 'safety')),
    object_key TEXT NOT NULL DEFAULT '',
    archive_format TEXT NOT NULL DEFAULT '',
    checksum TEXT NOT NULL DEFAULT '',
    compressed_size INTEGER NOT NULL DEFAULT 0 CHECK(compressed_size >= 0),
    engine TEXT NOT NULL CHECK(engine IN ('postgresql', 'mysql', 'mariadb', 'mongodb', 'redis', 'valkey')),
    database_name TEXT NOT NULL,
    engine_version TEXT NOT NULL,
    encryption_algorithm TEXT NOT NULL DEFAULT '',
    encrypted_data_key BLOB,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL DEFAULT '',
    completed_at TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(operation_id) REFERENCES database_operations(id) ON DELETE SET NULL,
    FOREIGN KEY(database_instance_id) REFERENCES database_instances(id) ON DELETE SET NULL,
    FOREIGN KEY(destination_id) REFERENCES backup_destinations(id) ON DELETE SET NULL
);

CREATE TABLE database_operations (
    id TEXT PRIMARY KEY,
    service_id TEXT NOT NULL,
    database_instance_id TEXT,
    operation_type TEXT NOT NULL
        CHECK(operation_type IN ('provision', 'start', 'stop', 'restart', 'backup', 'restore', 'rotate_credentials', 'upgrade', 'delete', 'restore_deleted', 'purge')),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK(status IN ('queued', 'running', 'success', 'failed', 'cancelled')),
    progress_step TEXT NOT NULL DEFAULT '',
    progress_percent INTEGER NOT NULL DEFAULT 0 CHECK(progress_percent BETWEEN 0 AND 100),
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
    FOREIGN KEY(service_id) REFERENCES services(id) ON DELETE CASCADE,
    FOREIGN KEY(database_instance_id) REFERENCES database_instances(id) ON DELETE CASCADE
);

CREATE TABLE database_restore_jobs (
    operation_id TEXT PRIMARY KEY,
    backup_id TEXT NOT NULL,
    target_instance_id TEXT NOT NULL,
    safety_backup_id TEXT,
    mode TEXT NOT NULL CHECK(mode IN ('new_service', 'replace_current')),
    status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued', 'running', 'success', 'failed')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(operation_id) REFERENCES database_operations(id) ON DELETE CASCADE,
    FOREIGN KEY(backup_id) REFERENCES database_backups(id) ON DELETE RESTRICT,
    FOREIGN KEY(safety_backup_id) REFERENCES database_backups(id) ON DELETE RESTRICT,
    FOREIGN KEY(target_instance_id) REFERENCES database_instances(id) ON DELETE CASCADE
);

CREATE TABLE database_upgrade_jobs (
    operation_id TEXT PRIMARY KEY,
    database_instance_id TEXT NOT NULL,
    engine_version TEXT NOT NULL,
    previous_image_ref TEXT NOT NULL,
    target_image_ref TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued', 'running', 'success', 'failed', 'rolled_back')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(operation_id) REFERENCES database_operations(id) ON DELETE CASCADE,
    FOREIGN KEY(database_instance_id) REFERENCES database_instances(id) ON DELETE CASCADE
);

CREATE INDEX idx_services_type ON services(service_type);
CREATE INDEX idx_database_instances_environment ON database_instances(environment_id, status);
CREATE INDEX idx_database_instances_purge ON database_instances(purge_after) WHERE deleted_at <> '';
CREATE INDEX idx_database_bindings_consumer ON database_bindings(consumer_service_id);
CREATE INDEX idx_database_backups_instance ON database_backups(database_instance_id, created_at DESC);
CREATE INDEX idx_database_operations_service ON database_operations(service_id, created_at DESC);
CREATE INDEX idx_database_restore_jobs_backup ON database_restore_jobs(backup_id, created_at DESC);
CREATE INDEX idx_database_upgrade_jobs_instance ON database_upgrade_jobs(database_instance_id, created_at DESC);
