---
title: Environment variables
description: Server and operator configuration via HOSTFORGE_* environment variables.
slug: environment-variables
group: Reference
order: 21
---

The **`hostforge-server`** process reads configuration from the environment (and flags). This table is condensed from the main **README** — treat that file as canonical when in doubt.

## Core server

| Variable | Purpose |
|----------|---------|
| `HOSTFORGE_LISTEN` | Listen address for API, UI, auth, webhooks (default `:8080`). |
| `HOSTFORGE_API_TOKEN` | **Required.** Bearer token for management APIs and UI login password material. |
| `HOSTFORGE_SESSION_SECRET` | **Required.** HMAC key for signed cookies (≥ **16** chars). |
| `HOSTFORGE_SESSION_COOKIE_NAME` | Cookie name (default `hostforge_session`). |
| `HOSTFORGE_SESSION_TTL_MINUTES` | Session lifetime (default `720`). |
| `HOSTFORGE_SESSION_COOKIE_SECURE` | If `true`, set `Secure` on cookies (use behind HTTPS). |

## Webhooks

| Variable | Purpose |
|----------|---------|
| `HOSTFORGE_WEBHOOK_SECRET` | **Required.** GitHub `X-Hub-Signature-256` verification. |
| `HOSTFORGE_WEBHOOK_BASE_PATH` | Route path (default `/hooks/github`). |
| `HOSTFORGE_WEBHOOK_MAX_BODY_BYTES` | Max body size (default `1048576`). |
| `HOSTFORGE_WEBHOOK_ASYNC` | If `true`, accept async processing semantics (`202`). |
| `HOSTFORGE_WEBHOOK_RATE_LIMIT_PER_MINUTE` | Per-IP ceiling (**must be > 0**). |

## Caddy

| Variable | Purpose |
|----------|---------|
| `HOSTFORGE_CADDY_BIN` | Caddy executable (default `caddy`). |
| `HOSTFORGE_CADDY_GENERATED_PATH` | Snippet output path (default `<data-dir>/caddy/hostforge.caddy`). |
| `HOSTFORGE_CADDY_ROOT_CONFIG` | Root Caddyfile for `validate` / `reload` (**required for sync**). |
| `HOSTFORGE_SYNC_CADDY` | If `true`, sync after successful deploy. |
| `HOSTFORGE_DOMAIN_SYNC_AFTER_MUTATE` | Sync after domain mutations when root config is set (default `true`). |

## Health checks (cutover)

| Variable | Purpose |
|----------|---------|
| `HOSTFORGE_HEALTH_PATH` | HTTP path to probe (default `/`). |
| `HOSTFORGE_HEALTH_TIMEOUT_MS` | Per-attempt timeout (default `3000`). |
| `HOSTFORGE_HEALTH_RETRIES` | Attempt count (default `10`). |
| `HOSTFORGE_HEALTH_INTERVAL_MS` | Delay between attempts (default `1000`). |
| `HOSTFORGE_HEALTH_EXPECTED_MIN` / `HOSTFORGE_HEALTH_EXPECTED_MAX` | Accepted HTTP status range (default `200`–`399`). |

## DNS guidance

| Variable | Purpose |
|----------|---------|
| `HOSTFORGE_DNS_SERVER_IPV4` | Fixed IPv4 for UI/CLI hints (skips auto-detect). |
| `HOSTFORGE_DNS_SERVER_IPV6` | Fixed IPv6 for AAAA hints. |
| `HOSTFORGE_DNS_DETECT_URL` | Plain-text IPv4 discovery URL. |
| `HOSTFORGE_DNS_DETECT_IPV6_URL` | Plain-text IPv6 discovery URL. |
| `HOSTFORGE_DNS_DETECT_TIMEOUT_MS` | Discovery timeout (default `2500`). |

## Project env encryption (optional)

| Variable | Purpose |
|----------|---------|
| `HOSTFORGE_ENV_ENCRYPTION_KEY` | Base64-encoded **32 raw bytes** (AES-256). Required for `/api/projects/:id/env` and for injecting stored env at deploy time. |

## Optional cert poll

| Variable | Purpose |
|----------|---------|
| `HOSTFORGE_CADDY_CERT_POLL_INTERVAL_SEC` | If `>0`, periodic read-only admin probe + optional leaf scan. |
| `HOSTFORGE_CADDY_ADMIN` | Admin API base (default `http://127.0.0.1:2019`). |
| `HOSTFORGE_CADDY_STORAGE_ROOT` | On-disk Caddy storage root for richer summaries. |
