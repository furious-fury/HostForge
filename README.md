# HostForge

HostForge is a private, GitHub-first application platform for a single Linux VPS. Its browser UI and authenticated API manage applications, production/staging environments, services, deployments, domains, encrypted variables, live logs, metrics, onboarding, and system diagnostics.

The active deployment path is **GitHub App -> Railpack/BuildKit -> Docker -> Caddy**. Legacy project APIs, PAT/SSH credential flows, project-owned persistence, and the operator CLI have been removed; `cmd/server` is the only entry point.

## Start here

- [Local development](./docs/development.md)
- [Management API v2](./docs/api-v2.md)
- [Operator guide](./docs/operator-guide.md)
- [Railpack/BuildKit decision](./docs/adr-0001-railpack-buildkit.md)

## Repository layout

| Path | Purpose |
| --- | --- |
| `cmd/server` | Authenticated API, UI serving, webhooks, logs, and workers |
| `web` | React/TanStack Query management UI |
| `internal` | SQLite, deployment, Docker, Caddy, GitHub App, and observability services |
| `site` | Public landing and operator documentation |

## Verification

```bash
go test ./...
npm --prefix web test
npm --prefix web run lint
npm --prefix web run build
npm --prefix site run build
```

## License

Apache-2.0. See [LICENSE](./LICENSE) and [NOTICE](./NOTICE). Commercial
use, including running a hosted version of HostForge for others, is
explicitly permitted. See [CONTRIBUTING.md](./CONTRIBUTING.md) for how
contributions are licensed.
