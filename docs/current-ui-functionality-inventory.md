# HostForge Current UI Functionality Inventory

This inventory describes the v2 interface and its live server contracts. The legacy Project UI and `/api/projects` contract are removed.

## Shell and authentication

- `/login`: exchanges the management API token for an HTTP-only session cookie.
- Every application route, including `/`, is protected and preserves the intended URL on session expiry.
- The sidebar reports the live server version and system-check state.
- Logout clears the session and TanStack Query cache.
- Command search includes static platform destinations plus fetched applications and services.
- Theme and accent remain local browser preferences.

## Overview

Route: `/`

- Application, service, deployment, active-build, and failed-deployment totals.
- Host CPU, memory, root disk, and network samples.
- Docker, Caddy, and webhook health checks.
- Recent deployments joined to application and service names.
- Server-reported onboarding progress.
- Loading, unsupported sampler, empty, error, refresh, and populated states.

## Applications

Routes:

- `/applications`
- `/applications/new`
- `/applications/:applicationID`
- `/applications/:applicationID/settings`
- `/applications/:applicationID/activity`

Functions:

- List and search applications with live service health and latest deployment data.
- Create an application; production and staging are created by the server.
- Update name/description, archive/restore, and permanently delete with confirmation.
- View services and durable application events.

## Services and environments

Routes:

- `/applications/:applicationID/services`
- `/applications/:applicationID/services/new`
- `/applications/:applicationID/services/:serviceID`
- `/applications/:applicationID/services/:serviceID/settings`
- `/applications/:applicationID/services/:serviceID/environment`

Functions:

- Create services only from repositories exposed through a GitHub App installation.
- Discover installations, repositories, and branches.
- Configure production/staging branch and automatic deployment independently.
- Edit each environment binding from service settings using branches returned by the linked GitHub installation.
- Edit source, build, runtime, port, and health-check configuration.
- Deploy, stop, restart, and delete services with mutation states and confirmation.

## Deployments and logs

Routes:

- `/deployments`
- `/deployments/:deploymentID`
- Application- and service-scoped deployment routes.

Functions:

- Server-side application, service, environment, status, trigger, branch, date, cursor, and limit filtering; the UI exposes compact status/trigger filters and cursor paging.
- Exact-commit redeploy.
- Cancellation while queued or building.
- Auditable rollback creates a new deployment with `rollback_of`; history is never mutated.
- Recorded deployment stages, resumable live build WebSockets, and downloadable completed log snapshots.
- Runtime WebSocket logs use authenticated reconnect and bounded buffering.

## Domains and variables

Routes:

- Application/service `domains` and `environment` tabs.

Functions:

- Domain create/edit/delete with target service, DNS/certificate state, and Caddy validation-before-reload.
- Environment-scoped application variables and service overrides.
- Values are encrypted and never returned; only last-four metadata is rendered.
- Delete and replacement actions require confirmation; `.env` imports validate all lines before mutation and report partial failures.

## Observability and status

Routes:

- `/observability`
- `/status`

Functions:

- Host snapshot/history, HTTP requests, deployment steps, platform events, and service metrics.
- Real time ranges and filters; unsupported and stale samples are explicit.
- System Status is diagnostic-only and exposes no Docker/Caddy restart controls.

## Settings and onboarding

Routes:

- `/settings`
- `/onboarding`

Functions:

- Environment-managed build, session, storage, network, DNS, webhook, and Caddy configuration.
- Safe public-IP detection, Caddy validation, and managed-route synchronization.
- GitHub manifest creation/exchange and installation readiness.
- DNS verification and permanent control-plane HTTPS cutover.
- Unsupported account/password, personal-token, SSH-key, and daemon-restart controls are not rendered.

## Documentation

Route: `/docs`

The hub is static operator reference content. It contains no fake server state or dead placeholder navigation.
