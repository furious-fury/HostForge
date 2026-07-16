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
