-- Attribute management request spans to the v2 hierarchy when route context is
-- available. Empty values remain valid for platform-wide endpoints.
ALTER TABLE http_requests ADD COLUMN application_id TEXT NOT NULL DEFAULT '';
ALTER TABLE http_requests ADD COLUMN service_id TEXT NOT NULL DEFAULT '';
ALTER TABLE http_requests ADD COLUMN environment_id TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_http_requests_service_started ON http_requests(service_id, started_at DESC);
CREATE INDEX idx_http_requests_environment_started ON http_requests(environment_id, started_at DESC);
