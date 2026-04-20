---
title: Installation
description: Build and install HostForge from a repository clone; optional systemd on Linux.
slug: installation
group: Getting Started
order: 2
---

## Prerequisites

- **Go** 1.22+
- **Git**, **Nixpacks** on `PATH`
- **Docker Engine** for image-based deploys
- For the server UI: **Node** to build `web/dist` (or ship prebuilt assets)

## Production install (from clone)

From the repository root on the build host:

```bash
./scripts/install.sh
```

This builds **`hostforge`** (`cmd/cli`) and **`hostforge-server`** (`cmd/server`) and installs them under **`PREFIX/bin`** (default **`/usr/local/bin`**). Re-run anytime; binaries are replaced idempotently.

### Optional systemd (Linux, root)

```bash
sudo ./scripts/install.sh --with-systemd
```

Creates user **`hostforge`**, data dir **`/var/lib/hostforge`**, seeds **`/etc/hostforge/hostforge.env`** from `scripts/hostforge-server.env.example` **only if** it does not exist, installs **`/etc/systemd/system/hostforge-server.service`**, runs **`daemon-reload`** and **`enable`**.

Edit secrets in **`/etc/hostforge/hostforge.env`**, then:

```bash
sudo systemctl start hostforge-server
```

### Installer flags

- **`--prefix`** — install path for binaries
- **`--data-dir`** — default data directory hint for docs (see installer output)
- **`--with-systemd`** — layout above
- **`--skip-build`** — install existing binaries only

If the **`docker`** group exists, **`hostforge`** is added so the service can use the Docker socket.

## Caddy

The install script **does not** install Caddy. Install Caddy separately, open **80/443**, and reverse-proxy to HostForge (e.g. `127.0.0.1:8080`) when exposing the UI or TLS-terminated webhooks.

## Environment before first server start

Copy `scripts/hostforge-server.env.example` and set at least:

- **`HOSTFORGE_API_TOKEN`**
- **`HOSTFORGE_SESSION_SECRET`** (≥16 characters)
- **`HOSTFORGE_WEBHOOK_SECRET`**
- **`HOSTFORGE_WEBHOOK_RATE_LIMIT_PER_MINUTE`** (must be `> 0`)

Set **`HOSTFORGE_DATA_DIR`** to a writable directory. See [Environment variables](/docs/environment-variables) for the full table.
