---
title: Domains and Caddy
description: Service-targeted domains, DNS ownership, and safe route synchronization.
slug: domains-and-caddy
group: Concepts
order: 12
---

A domain belongs to an **application environment** and targets one **service**. Caddy routes it to that service-environment binding's explicit `active_deployment_id`; HostForge never chooses a release by an unrelated global “latest success.”

Create, edit, and remove domains in the UI or v2 management API. HostForge validates the hostname, stores certificate observations, and reports Caddy synchronization outcomes. Operators still own registrar DNS and must point the hostname at the server.

HostForge writes a generated Caddy fragment and validates the root configuration before reload. Important settings include:

- `HOSTFORGE_CADDY_ROOT_CONFIG`
- `HOSTFORGE_CADDY_GENERATED_PATH`
- `HOSTFORGE_CADDY_BIN`
- `HOSTFORGE_SYNC_CADDY`
- `HOSTFORGE_DOMAIN_SYNC_AFTER_MUTATE`

System Status is diagnostic-only; it does not expose Docker or Caddy daemon restart controls.
