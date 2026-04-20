---
title: Domains and Caddy
description: Registering hostnames, DNS handoff, generated snippets, and sync semantics.
slug: domains-and-caddy
group: Concepts
order: 12
---

## Mental model

- **HostForge** stores **which hostname** belongs to **which project** in SQLite (`domains` table).
- **Caddy** terminates TLS and **`reverse_proxy`s** to **`127.0.0.1:<host_port>`** for the **latest successful deployment’s** container for that project.
- HostForge **does not** log in to registrars or DNS providers; operators (or app owners) create **`A` / `AAAA`** records in **their** DNS panel.

## Registering a hostname

Use the CLI (or the UI/API equivalent):

```bash
hostforge domain add --domain app.example.com https://github.com/org/repo.git
```

Use an HTTPS clone URL consistent with how the project is stored so webhook matching stays reliable.

## Caddy integration (v1)

HostForge writes a **generated fragment** under the data directory (default `<data-dir>/caddy/hostforge.caddy`) and runs:

- **`caddy validate --config <root>`**
- **`caddy reload --config <root>`**

against a **root Caddyfile you maintain** that **`import`s** the fragment. **v1 does not drive Caddy’s Admin API.**

Required / common env vars:

- **`HOSTFORGE_CADDY_ROOT_CONFIG`** — root Caddyfile path (required for sync).
- **`HOSTFORGE_CADDY_GENERATED_PATH`** — optional override for the snippet path.
- **`HOSTFORGE_CADDY_BIN`** — optional binary name (default `caddy`).
- **`HOSTFORGE_SYNC_CADDY`** — if `true`, sync after successful deploy when domains exist.

## DNS hints

The UI and CLI print **suggested DNS rows** using auto-detected public IPv4/IPv6, with optional overrides:

- `HOSTFORGE_DNS_SERVER_IPV4`
- `HOSTFORGE_DNS_SERVER_IPV6`

Until **`dig`** shows your server IP for the hostname, browsers will keep hitting old parking pages or the wrong origin.

## Domain sync after edits

When **`HOSTFORGE_DOMAIN_SYNC_AFTER_MUTATE`** is `true` (default) and **`HOSTFORGE_CADDY_ROOT_CONFIG`** is set, domain **add/edit/delete** paths can trigger a **Caddy sync** so the snippet matches SQLite.

## Further reading

- Main README: **Caddy and public HTTPS** (firewall, ACME, operator checklist).
- Optional cert polling: `docs/caddy-cert-poll.md` in the main repository.
