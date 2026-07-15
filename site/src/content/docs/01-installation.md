---
title: Installation
description: Build and configure HostForge with Railpack, BuildKit, Docker, and Caddy.
slug: installation
group: Getting Started
order: 2
---

## Prerequisites

- Go 1.25 or newer
- Git and Docker Engine
- Railpack at the configured pinned version
- `buildctl` plus a reachable BuildKit daemon
- Node.js/npm to build `web/dist`
- Caddy for public HTTPS routing

## Install

From the repository root:

```bash
./scripts/install.sh
```

For the managed Linux service layout:

```bash
sudo ./scripts/install.sh --with-systemd
```

The installer places binaries under `/usr/local/bin` by default. The systemd option creates the `hostforge` user, `/var/lib/hostforge`, `/etc/hostforge/hostforge.env`, and `hostforge-server.service`.

## Required configuration

Set the management/session/webhook secrets and writable data directory in the environment file. The active builder also requires:

```bash
HOSTFORGE_RAILPACK_ENABLED=true
HOSTFORGE_RAILPACK_BIN=/usr/local/bin/railpack
HOSTFORGE_RAILPACK_VERSION=v0.23.0
HOSTFORGE_RAILPACK_FRONTEND_IMAGE=ghcr.io/railwayapp/railpack-frontend@sha256:<digest>
HOSTFORGE_BUILDKIT_BIN=/usr/local/bin/buildctl
HOSTFORGE_BUILDKIT_ADDRESS=unix:///run/buildkit/buildkitd.sock
```

The frontend image must use an immutable digest. Configure Caddy's root file to import HostForge's generated route fragment, then start `hostforge-server`.
