# HostForge Management API v2

All management endpoints require the existing session cookie. Errors use `{"status":"error","error":"code","message":"optional","fields":{}}`.

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

## Domains and variables

- Domain CRUD: `/api/applications/:applicationID/environments/:environmentID/domains[/:domainID]`
- Variable CRUD: `/api/applications/:applicationID/environments/:environmentID/variables[/:variableID]`

Domain creation and updates require a target `service_id` bound to the selected application environment. Create, update, and delete restore database state if Caddy validation or synchronization fails. Variable creation may include `service_id` for an override. Secret plaintext is never returned; responses expose only metadata and `value_last4`.

## Events, observability, and metrics

- `GET /api/events`
- `GET /api/observability/summary`
- `GET /api/observability/requests`
- `GET /api/observability/deploy-steps`
- `GET /api/services/:serviceID/environments/:environmentID/metrics`

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
