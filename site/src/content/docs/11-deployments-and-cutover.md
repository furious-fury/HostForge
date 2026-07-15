---
title: Deployments and cutover
description: Service-environment release lifecycle, cancellation, rollback, and failure behavior.
slug: deployments-and-cutover
group: Concepts
order: 11
---

## Lifecycle

- `QUEUED`: accepted and cancellable.
- `BUILDING`: cloning, planning, building, starting, or health-checking; cancellation remains available.
- `SUCCESS`: healthy and promoted to the service environment's active release.
- `FAILED`: a material stage failed; the previous active release remains selected.
- `CANCELLED`: the operator cancelled a queued/building attempt.

Every deployment records its service, environment, trigger, actor, exact commit, stages, builder metadata, and optional rollback source.

## Cutover guarantees

The previous active container remains running while the candidate builds and passes health checks. HostForge promotes the candidate before rendering Caddy routes. If validation/reload fails, it restores the previous active deployment, marks the candidate failed, and removes the candidate container.

Redeploy creates a new deployment for the same exact commit. Rollback accepts a successful historical deployment and creates a new auditable deployment referencing it; history is never rewritten.

Build and runtime logs support authenticated catch-up and live WebSocket streaming. Navigating away does not cancel a deployment.
