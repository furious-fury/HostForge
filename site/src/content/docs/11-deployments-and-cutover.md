---
title: Deployments and cutover
description: Deployment statuses, candidate-first rollout, health checks, and failure behavior.
slug: deployments-and-cutover
group: Concepts
order: 11
---

## Deployment lifecycle

Deployments use a fixed set of statuses in SQLite:

- **`QUEUED`** — accepted, not yet building.
- **`BUILDING`** — clone / Nixpacks / image / container startup in progress.
- **`SUCCESS`** — healthy (and Caddy sync when required) completed; this is what public routes prefer.
- **`FAILED`** — build, health, or sync failure; error message stored on the row.

There is **no distinct `LIVE` state** in v1: the latest **`SUCCESS`** deployment for a project is what routing and the UI treat as “current”.

## Candidate-first cutover

HostForge keeps the **previous successful** container running while a **new candidate** is built and started on a **new host port**.

1. Start the **new** container on a **new** published port.
2. Probe **`127.0.0.1:<new_port>`** with **`HOSTFORGE_HEALTH_*`** settings (path, timeouts, retries, expected status range).
3. If health passes, optionally run **Caddy sync** so registered domains reverse-proxy to the new upstream.
4. Only then mark the deployment **`SUCCESS`** and **stop/remove** the previous container.

## Failure semantics

- **Build failure:** candidate deployment → **`FAILED`**; prior **`SUCCESS`** deployment and container remain serving.
- **Health failure:** same — route stays on the old upstream.
- **Caddy sync failure (when sync is required):** candidate **`FAILED`**; old route and container remain.

This matches the PRD intent: failed promotion must not take production offline on a single-node install.

## Configuration knobs

See [Environment variables](/docs/environment-variables) for:

- `HOSTFORGE_HEALTH_PATH`
- `HOSTFORGE_HEALTH_TIMEOUT_MS`
- `HOSTFORGE_HEALTH_RETRIES`
- `HOSTFORGE_HEALTH_INTERVAL_MS`
- `HOSTFORGE_HEALTH_EXPECTED_MIN` / `HOSTFORGE_HEALTH_EXPECTED_MAX`
- `HOSTFORGE_SYNC_CADDY` / `-sync-caddy`

## Operator validation

End-to-end “no downtime while curling through Caddy” is tracked as a **manual smoke** in the project task list. See [Operations and HTTPS](/docs/operations-https) for the runbook pointer.
