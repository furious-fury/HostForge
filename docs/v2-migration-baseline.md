# HostForge v2 migration baseline

**Status:** Stage 0 inventory (2026-07-11)  
**Scope:** This is the tracked migration map for the current server/UI baseline. It deliberately does not change product behavior or the ignored local planning documents.

## Baseline decision

The safe migration baseline is the current rich upstream server/UI implementation at `62877e7` (`origin/main`, checked on 2026-07-11). The local `main` checkout and its locally available `origin/main` ref have the same commit and have no committed divergence in either direction.

Migration work starts from this commit; it must be delivered in small, forward-only changes and must not overwrite uncommitted user work. The ignored operator planning files remain local-only and are not part of the baseline or commit scope.

## Retained baseline inventory

| Area | Current baseline | Migration disposition |
| --- | --- | --- |
| Server/API | `cmd/server` serves the management API, static UI, webhook endpoint, request IDs, session/token authentication, project/deployment/domain APIs, and webhook-triggered deployment execution. | Keep but refactor |
| Web UI | `web/` is a Vite/React management UI with dashboard, projects, deployments, settings, live logs, and observability pages. | Keep but refactor |
| SQLite/repository | Embedded, versioned SQLite migrations in `internal/database/migrations`; `internal/repository` owns persistence for projects, deployments, domains, credentials, GitHub App data, and observability. | Keep but refactor |
| Deployment lifecycle | `internal/services/deploy.go` prepares worktrees, clones, builds an image, allocates a loopback port, starts a Docker container, health-checks it, persists state, and coordinates Caddy cutover/rollback. | Keep but refactor |
| Builder | `internal/nixpacks` and `internal/services/nixpacks_worktree.go` invoke the Nixpacks CLI, generate `nixpacks.toml`, and derive stack metadata. | Replace |
| Docker runtime | `internal/docker` allocates and binds published ports to `127.0.0.1`, and supplies container lifecycle/log access. | Keep as-is |
| Caddy runtime | `internal/caddy` renders generated reverse-proxy routes to loopback ports and validates/reloads Caddy; cert polling is available. | Keep but refactor |
| GitHub App | `internal/github/app`, repository storage, server routes, and UI settings support app configuration, installation lookup, and short-lived installation-token Git access. | Keep but refactor |
| Git source fallback | Per-project PAT and SSH deploy-key models, repository methods, resolver fallback, API routes, and UI panels remain active. | Remove |
| Logs/live logs | File-backed build logs, Docker runtime logs, resumable WebSocket streaming, and UI consumers are implemented. | Keep as-is |
| Observability/status | Host metric sampler, persisted HTTP/deploy-step observations, system status, and dashboard/UI views are implemented. | Keep but refactor |
| Tests | Unit tests cover database migrations/repository, Nixpacks planning/worktree rendering, Docker validation, Caddy, GitHub App, Git auth resolution, logs, metrics, and services. | Keep but refactor |

## Migration tracks

### Keep as-is

- Docker's loopback-only host bindings in `internal/docker/runtime.go` and loopback port allocation in `internal/docker/ports.go`. Internal health checks may continue to use these loopback ports.
- File-backed build logs and Docker container log streaming in `internal/logs`, `internal/deploylogs`, and `cmd/server/deployment_logs_live.go`.
- Embedded SQLite migration execution and the repository boundary. New v2 state should arrive through new, forward-only migrations and repository methods.
- Host sampling and persisted observability as a base for the operations UI; retain safe, read-only Docker/Caddy status rather than adding restart controls.

### Keep but refactor

- **Server/API:** retain `cmd/server` as the normal product surface, but move orchestration from route handlers and the former CLI into explicit services/workers. Replace the shared-token/admin-session model with the invite-only account, role, and GitHub login model.
- **Web UI:** retain its routes, data-query hooks, deployment timeline, logs, settings, and observability structure. Rework project flows around GitHub App repository selection and production/staging environments; remove raw access URLs and legacy credential controls.
- **SQLite/repository:** preserve migration discipline and the store boundary. Add durable users/memberships, environments, environment-scoped branches/variables/domains/active deployments, reserved project slugs, audit events, and builder metadata.
- **Caddy:** retain generated-config validation/reload and HTTPS routing. Change route ownership from a project/latest-deployment port to a persisted active environment route, and make absent platform/custom domains explicitly `unpublished`.
- **GitHub App:** make it required for source access, preserve encrypted private-key storage and short-lived installation tokens, and ensure webhook mapping targets an environment branch rather than the current single project branch.
- **Stack detection/icons:** preserve the generic `stack_kind`/`stack_label` UI concept and assets where useful, but make the producer builder-neutral; remove Nixpacks terminology and metadata coupling.

### Replace

- **Builder:** introduce a builder interface plus result/event model, then replace `internal/nixpacks` and Nixpacks-specific deploy/worktree code with a Railpack adapter that uses Railpack's production BuildKit frontend path. Do not make the Railpack CLI a normal production dependency.
- **Build fallback:** select a repository `Dockerfile` fallback in the builder contract and surface which path was selected in persisted build metadata and live logs.
- **Build operations design:** record an ADR before implementation covering BuildKit topology/frontend invocation, image export/loading into the runtime daemon, supported Railpack version pinning, cache keys/retention, build concurrency/cancellation, disk guardrails, and image/worktree cleanup.

### Remove

- **CLI surface:** after service/API equivalents are identified and used by the server worker, remove `cmd/cli`, CLI build/install/release paths, CLI documentation, and any tests tied only to commands. Installation, recovery, and validation remain scripts/systemd/operator documentation, not HostForge commands.
- **Nixpacks surface:** remove the `internal/nixpacks` package, generated `nixpacks.toml`, Nixpacks runtime fields, names, tests, documentation, dependencies, and Nixpacks-derived API/UI language once the Railpack builder owns their replacements.
- **PAT/SSH source access:** remove per-project PAT and SSH deploy-key persistence, resolver fallbacks, API endpoints, UI panels, and documentation. The v1 source path is GitHub App only.
- **Public port sharing:** remove API/UI `http://127.0.0.1:<port>` access links and any host-port-as-public-URL behavior. A healthy container with no platform/custom domain remains internally healthy but unpublished.
- **CLI/Nixpacks teaching:** replace README and `site/` installation, quickstart, architecture, deployment, CLI reference, and marketing text so the product no longer teaches the retired flow.

### Deferred

- Preview/PR environments, with independent routing, retention, and cleanup policy.
- Managed databases/volumes and their backup, restore, and attachment lifecycle.
- Generic Git providers and deploy keys outside GitHub App access.
- Multi-host scheduling, high availability, billing, and public self-service signup.
- Docker/Caddy restart controls; v1 operations UI should show status and diagnostics only.

## Target data and publishing model

- Create `production` and `staging` as first-class environments for every project. Each environment owns branch, variables, domains, active deployment, status, and publishing state; preview environments are out of scope.
- Generate and reserve a project slug at project creation. Keep it reserved until project deletion, including after failed deployments or renames.
- Continue binding application containers only to `127.0.0.1`. Caddy HTTPS hostnames are the only public application ingress.
- A deployment may be healthy before it is published. Do not return a public IP/port URL when no platform hostname or custom domain is configured.

## Sequencing and dependency boundaries

1. Add the builder abstraction and its contract tests before replacing the current deployment pipeline; keep Docker runtime, health checks, logs, and Caddy integration behind the contract.
2. Add project/environment schema and API/UI state before making webhook routing or Caddy routes environment-specific.
3. Complete GitHub App-only source selection and migration of existing credential behavior before deleting PAT/SSH storage and UI.
4. Delete `cmd/cli` only after reusable deploy/domain/log logic is callable through server services/workers.
5. Remove Nixpacks names and docs only as Railpack/BuildKit produces equivalent build status, stack metadata, and Dockerfile fallback behavior.

## Highest-risk areas

- **BuildKit integration:** image export/loading, cache isolation, cancellation, and cleanup must work with the Docker runtime without reintroducing a Railpack CLI dependency.
- **State migration:** converting project-scoped deployments/domains/branches to production/staging environment ownership needs forward-only SQLite migrations and careful preservation of active routes.
- **Ingress safety:** Caddy route changes must preserve validate-before-reload and old-route retention so a failed deploy cannot publish a raw port or take down a healthy release.
- **Credential removal:** PAT/SSH data may exist in SQLite and must be removed/redacted without breaking existing projects; GitHub App installation selection must be complete first.
- **CLI extraction:** command handlers may share orchestration assumptions with server paths, so delete only after service ownership and regression coverage are explicit.

## Recommended next commit

Add the builder interface with contract tests only (including Dockerfile fallback selection), following [ADR 0001](./adr-0001-railpack-buildkit.md). Do not switch the active deploy path, delete legacy surfaces, or alter SQLite data in that commit.
