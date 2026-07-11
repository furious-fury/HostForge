---
title: CLI reference
description: hostforge subcommands, flags, and environment parity with the server deploy path.
slug: cli-reference
group: Reference
order: 20
---

All examples assume a built **`hostforge`** binary on `PATH`.

## Global flags / env

- **`--data-dir`** — overrides the data root (default `./.hostforge`). Env: **`HOSTFORGE_DATA_DIR`**.
- **`--branch`** — optional branch; default is the remote’s default branch.
- **`--host-port`** — `-1` picks from a configured range, `0` uses an ephemeral port, `>0` pins a host port.
- **`--port-start` / `--port-end`** — host port range when `--host-port=-1`.
- **`--container-port`** — app port inside the container (default **`3000`**).

Equivalent env vars:

- `HOSTFORGE_HOST_PORT`
- `HOSTFORGE_PORT_START` / `HOSTFORGE_PORT_END`
- `HOSTFORGE_CONTAINER_PORT`

## `hostforge deploy`

```bash
hostforge deploy [flags] <repo_url>
```

Builds with Nixpacks, creates/runs a Docker container, persists SQLite state when configured.

## `hostforge domain`

```bash
hostforge domain add [flags] --domain <hostname> <repo_url>
hostforge domain remove [flags] (--id <domain_id> | --domain <hostname> <repo_url>)
hostforge domain edit [flags] --id <domain_id> --domain <new_hostname>
```

## `hostforge caddy sync`

Regenerates the HostForge Caddy fragment from SQLite and reloads Caddy against your **root** config.

```bash
hostforge caddy sync [flags]
```

## `hostforge validate`

```bash
hostforge validate docker|preflight
```

Quick operator checks for Docker reachability and host prerequisites.

## `hostforge version`

Prints the embedded semver (see `internal/version/VERSION` in the main repository).
