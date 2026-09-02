---
title: Installation
description: Build and configure HostForge with Railpack, BuildKit, Docker, and Caddy.
slug: installation
group: Getting Started
order: 2
---

## Install

On a fresh Ubuntu 24.04 host, as root:

```bash
curl -fsSL https://raw.githubusercontent.com/furious-fury/HostForge/main/scripts/bootstrap-ubuntu.sh | sudo bash
```

This provisions Docker, BuildKit, Railpack, and Caddy, then installs the latest HostForge release: a checksum-verified prebuilt binary and UI. Nothing is compiled on your server, so it needs neither the Go nor the Node toolchain. You are prompted once for an admin login secret.

Pin a specific release instead of the latest:

```bash
curl -fsSL https://raw.githubusercontent.com/furious-fury/HostForge/main/scripts/bootstrap-ubuntu.sh | sudo HOSTFORGE_VERSION=v0.9.0 bash
```

## Prerequisites

The bootstrapper installs everything it needs. It expects:

- Ubuntu 24.04
- Root access
- Ports 80, 443, and 5432 free

## Installing from source

To run unreleased code — testing a branch, or developing against a real host — install from source instead. This mode clones the repository and builds on the server, so it also installs Go, Node.js, and git:

```bash
curl -fsSL https://raw.githubusercontent.com/furious-fury/HostForge/main/scripts/bootstrap-ubuntu.sh | sudo bash -s -- --from-source
```

To install into an existing checkout directly, `scripts/install.sh --with-systemd` builds from that tree, and `--download-release` installs a published build into it instead.

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
