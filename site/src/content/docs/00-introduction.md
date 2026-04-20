---
title: Introduction
description: What HostForge is, how it fits on one machine, and how to read these docs.
slug: introduction
group: Getting Started
order: 1
---

HostForge is a **self-hosted PaaS** for a single machine: **Git → [Nixpacks](https://github.com/railwayapp/nixpacks) → Docker**, plus a management **API**, **browser UI**, **GitHub `push` webhooks**, optional **Caddy** for public TLS routing, and **SQLite** persistence.

## Who it is for

Operators who want Vercel-style ergonomics **on their own metal**: one host running Docker, a reverse proxy (typically Caddy on 80/443), and DNS pointed at that host.

## What ships in this repository

- **`hostforge` CLI** (`cmd/cli`) — deploy, domain management, `caddy sync`, `validate`, `version`.
- **`hostforge-server`** (`cmd/server`) — REST + embedded UI, webhooks, cookie-backed UI login, bounded observability (`deploy_steps`, `http_requests`).
- **Web UI** (`web/`) — projects, deployments (REST + WebSocket logs), domains, dashboard system/host panels, settings.

## Agent-friendly docs

Every doc page is also published as **raw Markdown** at `/docs/<slug>.md` after `npm run build`. A generated **`/llms.txt`** lists all pages for LLM crawlers; **`/llms-full.txt`** concatenates bodies for offline ingestion.

## Next steps

- [Installation](/docs/installation) — install binaries and optional systemd layout.
- [Quickstart](/docs/quickstart) — run the golden-path deploy and open the UI.
