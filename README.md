# HostForge

HostForge is a private, GitHub-first application platform for a Linux VPS. Its
normal product surface is the browser UI for source selection, deployments,
domains, logs, and host status.

The repository is migrating from its legacy CLI/Nixpacks baseline to Railpack
with BuildKit, GitHub App-only access, and production/staging environments. See
the [v2 migration baseline](./docs/v2-migration-baseline.md) for the current
implementation status.

## Start here

- [Local development](./docs/development.md)
- [Operator guide](./docs/operator-guide.md)
- [Railpack/BuildKit decision](./docs/adr-0001-railpack-buildkit.md)
- [Migration inventory](./docs/v2-migration-baseline.md)
- [Phase 1 validation runbook](./docs/operator-validation-phase1.md)
- [Marketing/site project](./site/README.md)

## Current status

- The server (`cmd/server`) and browser UI (`web/`) are the retained product
  baseline.
- SQLite, Docker loopback runtime, Caddy, live logs, GitHub App, and host
  observability are migration foundations.
- Railpack/BuildKit is feature-gated pending clean-Linux-VPS validation.
- The legacy CLI is scheduled for removal; do not build new workflows around it.

## Repository layout

| Path | Purpose |
| --- | --- |
| `cmd/server` | API, UI serving, webhooks, and workers |
| `web` | React management UI |
| `internal` | Services, SQLite repository, Docker/Caddy/GitHub integrations |
| `internal/railpack` | Feature-gated Railpack and BuildKit path |
| `docs` | Architecture, migration, development, and operator material |
| `site` | Public landing and documentation site |

## Verification

```bash
go test ./...
```

Frontend builds require installed dependencies:

```bash
npm --prefix web install
npm --prefix web run build
```

Private planning files remain ignored by design; do not stage or publish them.
