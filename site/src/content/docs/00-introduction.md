---
title: Introduction
description: HostForge's application, service, deployment, and host-management model.
slug: introduction
group: Getting Started
order: 1
---

HostForge is a private, self-hosted application platform for one Linux server: **GitHub App -> Railpack/BuildKit -> Docker -> Caddy**. The browser UI and authenticated management API own the product workflow; SQLite stores control-plane state and bounded observability.

## Resource model

- An **application** groups related services and owns production and staging environments.
- A **service** selects a GitHub App repository plus build, runtime, port, and health configuration.
- A **service-environment binding** selects the branch, automatic deployment behavior, desired runtime state, and active release.
- A **deployment** is an immutable build/run attempt for one service environment.
- A **domain** routes one application environment hostname to a selected service.

## Repository surfaces

- **`hostforge-server`** (`cmd/server`) serves the REST API, browser UI, GitHub webhooks, live logs, metrics, and system diagnostics.
- **Web UI** (`web/`) provides authenticated application, service, deployment, domain, variable, observability, onboarding, and settings workflows.
- **`hostforge` CLI** (`cmd/cli`) is intentionally operator-only: validation, Caddy synchronization, and version output.

Private repositories use GitHub App installation tokens only. PAT and SSH credential management are not supported.
