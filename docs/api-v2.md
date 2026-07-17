# HostForge Management API v2

All management endpoints require the existing session cookie. Errors use `{"status":"error","error":"code","message":"optional","fields":{}}`.

Collection fields are always JSON arrays. Empty applications, services, deployments, metrics, events, GitHub resources, domains, variables, and observability results return `[]`, never `null`. The frontend API adapter also normalizes nullable collection fields for safe upgrades from older server builds.

## Authentication and bootstrap

- `GET|POST|DELETE /auth/session`
- `GET|PATCH /api/onboarding`
- `GET /api/system/status`
- `GET /api/system/host/snapshot`
- `GET /api/system/host/history?points=120`

## Applications, environments, and services

- `GET|POST /api/applications`
- `GET|PATCH|DELETE /api/applications/:applicationID`
- `GET|POST /api/applications/:applicationID/environments`
- `PATCH /api/applications/:applicationID/environments/:environmentID`
- `GET|POST /api/applications/:applicationID/services`
- `GET|PATCH|DELETE /api/services/:serviceID`
- `GET|PATCH /api/services/:serviceID/environments/:environmentID`
- `GET /api/repositories/branches?repo_url=...&installation_id=...`

Every application is created with production and staging. Creating an additional environment also creates a binding for every existing service in one transaction. A migrated staging binding intentionally has no branch.

Application collection rows include `environment_health`, `service_count`, `healthy_service_count`, `domain_count`, and `latest_deployment`. Service detail includes environment binding state, the active release, persisted container state, domains, and the public Caddy URL when configured.

Creating a service, or changing its source through `PATCH`, verifies that `github_installation_id` identifies a known, active installation and that `repo_url` is present in the installation's live GitHub repository list. Changing an environment binding branch verifies that the branch still exists in that repository. Non-source service edits, unchanged bindings, and an empty staging branch do not require GitHub availability. Validation failures use stable `fields` entries with `github_installation_required`, `github_installation_not_found`, `github_installation_suspended`, `repository_not_accessible`, or `branch_not_accessible`; upstream GitHub failures return `502` without persisting the requested change.

## Database services

- `GET /api/database-engines`
- `POST /api/applications/:applicationID/database-services`
- `GET /api/services/:serviceID`
- `GET /api/database-operations/:operationID`
- `POST /api/database-instances/:instanceID/start`
- `POST /api/database-instances/:instanceID/stop`
- `POST /api/database-instances/:instanceID/restart`
- `POST /api/database-instances/:instanceID/rotate-credentials`
- `GET|POST /api/database-instances/:instanceID/upgrade`
- `GET /api/database-instances/:instanceID/logs?tail=200`
- `GET /api/database-instances/:instanceID/metrics`
- `DELETE /api/services/:serviceID`
- `POST /api/database-services/:serviceID/restore-deleted`
- `DELETE /api/database-services/:serviceID/purge`
- `GET|POST /api/backup-destinations`
- `PATCH /api/backup-destinations/:destinationID`
- `POST /api/backup-destinations/:destinationID/test`
- `DELETE /api/backup-destinations/:destinationID`
- `GET|PUT /api/database-instances/:instanceID/backup-policy`
- `GET|POST /api/database-instances/:instanceID/backups`
- `POST /api/database-backups/:backupID/restore`
- `DELETE /api/database-backups/:backupID`
- `GET|POST /api/database-instances/:instanceID/bindings`
- `PATCH /api/database-bindings/:bindingID`
- `DELETE /api/database-bindings/:bindingID`

Database creation accepts a server-catalog engine/version, one or more application environment IDs, a resource preset, and optional application-service connection bindings. It returns `202` with one durable provisioning operation per isolated environment instance. Clients cannot submit images, ports, volume paths, or Docker networking options.

The digest-pinned catalog provisions PostgreSQL 16–18, MySQL 8.4, MariaDB 11.4, MongoDB 8.0, Redis 8.4/8.8, and Valkey 8.1/9.0. Each engine uses its own initialization, readiness, persistence, and credential-rotation commands while sharing the same private-network and retained-volume safety model.

Instances use one encrypted credential record, named volume, network alias, and resource allocation per environment. Application containers receive managed connection URLs only when an explicit binding exists in the same environment. Binding writes reject duplicate managed ownership and existing application-variable conflicts unless the caller explicitly sends `replace_existing: true`; that confirmation is scoped to the consumer service, environment, and variable key. Plaintext passwords and full connection URLs are never returned by the API or written to operation events.

Database ports are not published on the VPS. Runtime start, stop, and restart requests return `202` and are processed by the durable operation worker. Deletion immediately removes owned database containers and connection bindings, retains volumes for seven days, and returns `purge_after`. Restore is available within that window. A background worker permanently removes only volumes whose HostForge ownership and database-instance labels match.

Cloudflare R2 destinations derive the provider endpoint from the account ID and use region `auto`; generic storage requires an HTTPS S3-compatible endpoint and region. Generic S3 additionally accepts `server_side_encryption` as empty, `AES256`, or `aws:kms`; `aws:kms` requires `sse_kms_key_id`. R2 clears these provider-specific fields. Both flows perform a write/read/delete bucket probe before encrypted credentials are persisted. Database policies validate standard five-field cron expressions and IANA timezones. Manual and scheduled backups run as short-lived, network-private containers, stream directly through gzip and chunked AES-256-GCM into multipart R2/S3 upload, verify the remote object size, and retain only the wrapped per-backup data key in SQLite. Expired objects are deleted by a retryable retention worker.

Backup restore defaults to `new_service`, which provisions an isolated target with new credentials before applying the encrypted logical backup. `replace_current` requires `target_instance_id` and a `confirmation` exactly matching the target service name. HostForge queues a fresh safety backup first, stops bound application containers during replacement, and automatically restores the safety backup if the requested restore fails. Redis and Valkey use an offline retained-volume RDB replacement; the other engines use their logical restore clients. Both modes return `202` with a durable restore operation.

Database operations are claimed with renewable database-backed leases and expose an `attempt_count` without exposing lease-owner internals. Expired leases atomically requeue the operation and its backup, restore, or upgrade companion record; each backup is linked to its owning operation so retries cannot select a different queued attempt. Backup and restore queue admission is globally bounded by `HOSTFORGE_DATABASE_TRANSFER_MAX_PER_HOUR`; an exhausted limit returns `429` with `database_transfer_rate_limited`. Runtime execution is bounded separately by `HOSTFORGE_DATABASE_OPERATION_CONCURRENCY`.

## Deployments and runtime

- `GET /api/deployments`
- `GET /api/deployments/:deploymentID`
- `POST /api/services/:serviceID/environments/:environmentID/deployments`
- `POST /api/deployments/:deploymentID/redeploy`
- `POST /api/deployments/:deploymentID/cancel`
- `POST /api/deployments/:deploymentID/rollback`
- `POST /api/services/:serviceID/environments/:environmentID/restart`
- `POST /api/services/:serviceID/environments/:environmentID/stop`
- `GET /api/deployments/:deploymentID/logs`
- `GET /api/deployments/:deploymentID/logs/live`
- `GET /api/deployments/:deploymentID/steps`

Deployment filters are `application_id`, `service_id`, `environment_id`, `status`, `trigger`, `branch`, `date_from`, `date_to`, `cursor`, and `limit`. Collection responses include `next_cursor`. The cursor is an opaque deployment ID and preserves deterministic `(created_at, id)` ordering when timestamps are equal. A deployment stores its branch at queue time, so historical branch filters do not change when a binding is edited.

Rollback accepts only a successful deployment with a recorded commit. It queues a new health-checked release and records the source in `rollback_of`.

The authenticated live-log WebSocket defaults to JSON frames. Its `hello` frame includes `deployment_id`, `application_id`, `service_id`, `environment_id`, source, resumability, and byte cursor metadata. Build clients reconnect with `cursor`; a truncated or rotated file emits `resync`. When a deployment becomes `SUCCESS`, `FAILED`, or `CANCELLED`, the server catches up any final bytes and emits an `end` frame containing the terminal status and final EOF offset. Container streams explicitly report `resume: false`.

## Domains and variables

- Domain CRUD: `/api/applications/:applicationID/environments/:environmentID/domains[/:domainID]`
- Variable CRUD: `/api/applications/:applicationID/environments/:environmentID/variables[/:variableID]`

Domain creation and updates require a target `service_id` bound to the selected application environment. Create, update, and delete restore database state if Caddy validation or synchronization fails. Variable creation may include `service_id` for an override. Secret plaintext is never returned; responses expose only metadata and `value_last4`.

Domain collection responses include DNS record guidance and optional certificate-poll metadata. Pass `check_dns=1` on a domain collection request to perform operator-triggered public A-record checks. Each check reports `ok`, `pending`, `unknown`, or `lookup_error`, the expected server IPv4, and the IPv4 addresses observed. Domain mutation responses include `caddy_sync`; a failed Caddy validation or reload returns `502 caddy_sync_failed` and the database change is rolled back.

## Events, observability, and metrics

- `GET /api/events`
- `GET /api/observability/summary`
- `GET /api/observability/requests`
- `GET /api/observability/deploy-steps`
- `GET /api/services/:serviceID/environments/:environmentID/metrics?points=360`

Service metrics are sampled by the server every 10 seconds for each running active container and retained in SQLite, capped at 720 samples per service/environment binding. Reading the endpoint does not trigger collection. Responses include the newest `sample`, oldest-first `samples`, `sample_interval_seconds`, and stale metadata when the service is stopped or collection is delayed.

Event filters are `application_id`, `service_id`, `type`, `date_from`, `date_to`, `cursor`, and `limit`.

Request filters are `application_id`, `service_id`, `environment_id`, `method`, `status_class`, `date_from`, `date_to`, `cursor`, and `limit`. Valid status classes are `success`, `client_error`, and `server_error`.

Deployment-step filters are `application_id`, `service_id`, `environment_id`, `status`, `date_from`, `date_to`, `cursor`, and `limit`. Event, request, and deployment-step collection responses include a stable ID-based `next_cursor`.

Persisted HTTP request records include resolved `application_id`, `service_id`, and `environment_id` when the request targets a v2 resource.

## GitHub App and settings

- `GET|DELETE /api/github/app`
- `POST /api/github/app/manifest`
- `POST /api/github/app/manifest/exchange`
- `GET /api/github/installations`
- `POST /api/github/installations/sync`
- `GET /api/github/installations/:installationID/repositories`
- `GET /api/settings`
- `POST /api/settings/actions/caddy-validate`
- `POST /api/settings/actions/caddy-sync`
- `POST /api/settings/actions/refresh-status`
- `POST /api/settings/actions/detect-public-ipv4`

GitHub App is the only private-source credential model. There are no public project, PAT, or SSH management endpoints.

## Migration safety

Before the application/service cutover is applied to an existing SQLite database, HostForge creates `<database>.pre-application-model.bak`. Each migration and its schema-version record run in one transaction. Migration tests cover populated legacy deployments, containers, domains, secrets, GitHub installation references, and observability rows.
