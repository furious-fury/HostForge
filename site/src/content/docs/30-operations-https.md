---
title: Operations and HTTPS
description: Firewall expectations, operator validation phases, and where to record smoke-test evidence.
slug: operations-https
group: Operations
order: 30
---

## Firewall and exposure

- **Caddy** (or another edge proxy) should own inbound **TCP 80** and **443** on the public interface for ACME HTTP-01 and browser traffic.
- The **HostForge API** port should **not** be world-exposed unless you intend it: bind to loopback and reverse-proxy admin paths, or restrict by source IP / VPN.

## Operator validation (phase 1)

The main repository ships a staged runbook: **`docs/operator-validation-phase1.md`**.

Sections:

1. **§1.1 — Docker runtime proof** — recorded **PASS** (example: WSL2 + Docker Engine, 2026-04-19 in the project task list).
2. **§1.2 — Public HTTPS + restarts** — **Pending** until exercised on a VPS with real DNS and Caddy.
3. **§1.3 — Zero-downtime redeploy** — **Pending** until observed under load or a scripted curl loop through Caddy.

Completing §1.2 / §1.3 closes important **checkbox exits** in `task_list.md` that code alone cannot prove.

## TLS responsibility

**TLS issuance and renewal** are handled by **Caddy automatic HTTPS** (typically Let’s Encrypt). HostForge stores **`ssl_status`** in SQLite based on **validate/reload** outcomes, not by re-implementing ACME.

Optional **`HOSTFORGE_CADDY_CERT_POLL_INTERVAL_SEC`** mirrors operator-facing cert hints into SQLite without duplicating ACME state.

## Logs and long-lived connections

If you terminate TLS at Caddy, ensure **WebSocket upgrade** headers are forwarded for **`/api/deployments/*/logs/live`**, and proxy idle timeouts exceed your longest quiet build phases. The server emits JSON **heartbeats** on log streams to survive proxies that ignore WS ping frames.

## Incident checklist (short)

1. **`docker ps`** — is the candidate container up? Published port?
2. **HostForge deployment row** — status and `error_message`.
3. **`caddy validate`** with your root config — syntax errors in the imported fragment?
4. **`dig`** — does the public hostname still point at the old parking IP?
