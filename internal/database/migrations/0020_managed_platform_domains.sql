ALTER TABLE domains
ADD COLUMN kind TEXT NOT NULL DEFAULT 'custom'
CHECK(kind IN ('custom','platform'));

CREATE UNIQUE INDEX idx_domains_platform_service_environment
ON domains(service_id,environment_id)
WHERE kind='platform';
