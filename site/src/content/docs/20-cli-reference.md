---
title: CLI reference
description: Safe operator-only HostForge commands.
slug: cli-reference
group: Reference
order: 20
---

The CLI intentionally does not create applications, mutate domains, or deploy repositories. Those are authenticated server/API workflows.

## Caddy synchronization

```bash
hostforge caddy sync [-data-dir /var/lib/hostforge]
```

Regenerates service-environment routes from SQLite and runs the configured validate/reload flow.

## Host validation

```bash
hostforge validate docker
hostforge validate preflight
```

`docker` checks daemon reachability. `preflight` also verifies Git, Railpack, and BuildKit tooling.

## Version

```bash
hostforge version
```
