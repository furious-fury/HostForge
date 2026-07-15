-- Remove the legacy projects foreign key from domains so newly-created v2 services
-- can own routes without a shadow project row. project_id remains as a temporary
-- read bridge until all legacy project handlers are removed.
CREATE TABLE domains_v2 (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL DEFAULT '',
	application_id TEXT NOT NULL DEFAULT '',
	environment_id TEXT NOT NULL DEFAULT '',
	service_id TEXT NOT NULL DEFAULT '',
	domain_name TEXT NOT NULL UNIQUE,
	ssl_status TEXT NOT NULL DEFAULT 'PENDING',
	last_cert_message TEXT NOT NULL DEFAULT '',
	cert_checked_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

INSERT INTO domains_v2(
	id,project_id,application_id,environment_id,service_id,domain_name,
	ssl_status,last_cert_message,cert_checked_at,created_at,updated_at
)
SELECT
	id,project_id,application_id,environment_id,service_id,domain_name,
	ssl_status,last_cert_message,cert_checked_at,created_at,updated_at
FROM domains;

DROP TABLE domains;
ALTER TABLE domains_v2 RENAME TO domains;

CREATE INDEX idx_domains_application_environment
ON domains(application_id, environment_id, domain_name);
CREATE INDEX idx_domains_service_environment
ON domains(service_id, environment_id, domain_name);
