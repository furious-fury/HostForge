# HostForge

Self-hosted PaaS: deploy from Git to a single VPS. **Phase 0** implements the CLI spike: clone a repo and run [Nixpacks](https://github.com/railwayapp/nixpacks) with streamed logs.


## Prerequisites

- **Go** 1.22+
- **Git** (for upstream repos; the CLI uses [go-git](https://github.com/go-git/go-git) but host tooling may still matter)
- **Nixpacks CLI** on your `PATH` — [installation](https://github.com/railwayapp/nixpacks#installation)
- **Docker** is *not* required for Phase 0 when using `nixpacks build ... -o <dir>` (filesystem output)
- **Docker Engine** is required for Phase 1 `deploy`, because Nixpacks now emits a Docker image and HostForge starts a container
- Docker CLI env vars such as **`DOCKER_HOST`**, `DOCKER_TLS_VERIFY`, and `DOCKER_CERT_PATH` are honored by the Docker client

### Windows

Nixpacks is most reliable under **WSL2** or a **Linux** environment. Native Windows may work if `nixpacks` is installed and on `PATH`, but if builds fail unexpectedly, try WSL2. For Phase 1 runtime, ensure Docker Desktop is running and its daemon is reachable (or set `DOCKER_HOST` explicitly).

## Build

```bash
go build -o hostforge ./cmd/cli
```

## Usage

```bash
./hostforge deploy [flags] <repo_url>
./hostforge domain add [flags] --domain <hostname> <repo_url>
./hostforge caddy sync [flags]
./hostforge version
```

- **`<repo_url>`:** HTTPS URL only in Phase 0 (e.g. `https://github.com/org/repo`).
- **`--data-dir`:** Overrides the data root (default: `./.hostforge`). You can also set **`HOSTFORGE_DATA_DIR`**.
- **`--branch`:** Optional branch name; default is the remote’s default branch.
- **`--host-port`:** `-1` picks from range, `0` asks OS for ephemeral port, `>0` uses exact host port.
- **`--port-start` / `--port-end`:** Host port range used when `--host-port=-1`.
- **`--container-port`:** App port inside the container (default `3000`).

Equivalent env vars:

- `HOSTFORGE_HOST_PORT` (`-1`, `0`, `>0`)
- `HOSTFORGE_PORT_START` / `HOSTFORGE_PORT_END`
- `HOSTFORGE_CONTAINER_PORT`

Data layout:

- Worktrees: `<data-dir>/worktrees/<hash>/`
- Nixpacks output: `<data-dir>/builds/<hash>/`

## Golden path (manual test)

Use a small public Node app with a root `package.json`:

```bash
go run ./cmd/cli deploy https://github.com/heroku/node-js-getting-started
```

**Expected:** Git clone progress on stderr, Nixpacks logs on stdout/stderr, exit code **0**, and artifacts under `.hostforge/builds/<hash>/`.

**If clone fails:** Check network, URL, and private-repo access (Phase 0 does not configure Git credentials beyond your environment).

**If Nixpacks fails:** Run `nixpacks plan .` inside the worktree path printed in logs, or install/upgrade Nixpacks. Ensure sufficient disk and that the stack is supported by Nixpacks.

Phase 1 note: Phase 0 uses `-o` filesystem output for fast validation; Phase 1 switches to `nixpacks build . --name <image>` so `hostforge deploy` can build an image and run a container. Image tags use `hostforge/<worktree-slug>:<utc-build-id>`.

## Phase 3: Caddy (reverse proxy + TLS)

HostForge writes a **generated Caddyfile fragment** under the data directory and runs **`caddy validate`** / **`caddy reload`** against a **root** Caddyfile you maintain that **imports** that fragment (see [implementation_plan.md](./implementation_plan.md)). **v1 does not use Caddy’s Admin API.**

### Install and host networking

- Install [Caddy](https://caddyserver.com/docs/install) by package or static binary; use a **recent 2.x** release.
- Caddy must be able to bind **80** and **443** on the VPS for automatic HTTPS (Let’s Encrypt). If you bind elsewhere, adjust your root Caddyfile accordingly.
- **DNS:** point each public hostname (`domain add`) at this host with **`A`/`AAAA`** to the server’s public IP before TLS issuance will succeed.

### Environment (HostForge)

| Variable | Purpose |
|----------|---------|
| `HOSTFORGE_CADDY_BIN` | Caddy executable (default: `caddy`) |
| `HOSTFORGE_CADDY_GENERATED_PATH` | Where HostForge writes the snippet (default: `<data-dir>/caddy/hostforge.caddy`) |
| `HOSTFORGE_CADDY_ROOT_CONFIG` | Root Caddyfile path passed to `caddy validate` / `caddy reload` (required for sync) |
| `HOSTFORGE_SYNC_CADDY` | If `true`, run `caddy sync` after a successful deploy (same as `-sync-caddy`) |
| `HOSTFORGE_HEALTH_PATH` | HTTP path probed before promotion (default: `/`) |
| `HOSTFORGE_HEALTH_TIMEOUT_MS` | Per-request health timeout in milliseconds (default: `3000`) |
| `HOSTFORGE_HEALTH_RETRIES` | Number of health attempts before deploy fails (default: `10`) |
| `HOSTFORGE_HEALTH_INTERVAL_MS` | Delay between health attempts in milliseconds (default: `1000`) |
| `HOSTFORGE_HEALTH_EXPECTED_MIN` | Minimum accepted health status code (default: `200`) |
| `HOSTFORGE_HEALTH_EXPECTED_MAX` | Maximum accepted health status code (default: `399`) |
| `HOSTFORGE_LISTEN` | Server listen address for API/webhooks (default: `:8080`) |
| `HOSTFORGE_WEBHOOK_BASE_PATH` | Webhook route path (default: `/hooks/github`) |
| `HOSTFORGE_WEBHOOK_MAX_BODY_BYTES` | Max webhook body size in bytes (default: `1048576`) |
| `HOSTFORGE_WEBHOOK_ASYNC` | If `true`, accept webhook deploys with `202` and run in background |
| `HOSTFORGE_WEBHOOK_SECRET` | Optional shared-secret token expected in `X-HostForge-Token` |
| `HOSTFORGE_LOGS_DIR` | Optional override for deployment build logs directory (default: `<data-dir>/logs`) |

### HTTPS / ACME

TLS is handled by **Caddy automatic HTTPS** (typically Let’s Encrypt). Certificate storage and renewal are **Caddy’s responsibility** on disk (see upstream Caddy docs for data dirs and [staging CA](https://caddyserver.com/docs/caddyfile/options#acme-ca) for testing). HostForge records domain `ssl_status` in SQLite from **validate/reload** success or failure, not by parsing ACME events.

### Routing model

- Register a hostname with **`hostforge domain add --domain app.example.com <repo_url>`** (same repo URL/branch semantics as deploy).
- **`hostforge caddy sync`** regenerates the snippet from SQLite and reloads Caddy. Each domain maps to the **latest successful deployment’s** container **`host_port`** for that project.
- **`hostforge deploy ... -sync-caddy`** runs that sync after a good deploy so routes point at the new container without hand-editing config.

### Zero-downtime orchestration

Deploy now uses a candidate-first cutover sequence:

1. Keep previous successful container running.
2. Start a new candidate container on a new host port.
3. Probe `127.0.0.1:<new_port><health_path>` using the health env/flags above.
4. If health passes, run Caddy sync (when `-sync-caddy` is set or when the project has registered domains).
5. Mark deployment `SUCCESS` only after successful health + sync.
6. Stop and remove the previous container after route switch.

Failure behavior:

- Build/health/Caddy-sync failures mark the candidate deployment as `FAILED`.
- Previous successful deployment keeps serving traffic.
- Candidate container is cleaned up on health/sync failure.

## Phase 4: Webhooks (GitHub push)

Phase 4 adds an HTTP server that accepts GitHub `push` webhooks and runs the same deployment pipeline as `hostforge deploy`.

### Build and run

```bash
go build -o hostforge-server ./cmd/server
./hostforge-server -data-dir ./.hostforge -listen :8080
```

You can also set `HOSTFORGE_DATA_DIR` and `HOSTFORGE_LISTEN` instead of flags.

### Reachability and networking

- GitHub must be able to reach your webhook URL on a public IP or through a reverse proxy.
- If this service is bound directly, allow the server port in your firewall.
- If you terminate TLS at Caddy or another proxy, forward webhook requests to HostForge unchanged and preserve headers.

### Endpoint and GitHub configuration

- Default webhook URL path: `http(s)://<host>:<port>/hooks/github`.
- Register a GitHub webhook with:
  - Content type: `application/json`
  - Event selection: `push` events only
  - URL: your publicly reachable HostForge webhook endpoint
- HostForge matches incoming payloads by `repository.clone_url` + branch against existing projects in SQLite. For reliable matching, register projects with an explicit `-branch`.

### Request handling behavior

- Non-JSON, malformed, or unauthorized requests are rejected with clear `4xx` responses.
- Unsupported or ignorable events (for example non-`push`, tag refs, or branch mismatch) return `200` with an `ignored` response and do not create a deployment.
- Unknown projects return `404` and do not trigger any deploy work.
- By default (`HOSTFORGE_WEBHOOK_ASYNC=false`), webhook deploys run synchronously and return after deploy completion.
- With async mode enabled, HostForge returns `202 Accepted` after durable acceptance and runs deployment in the background.

### Security scope (MVP)

- MVP supports an optional shared-secret header check (`X-HostForge-Token`) via `HOSTFORGE_WEBHOOK_SECRET`.
- Full GitHub signature verification (`X-Hub-Signature-256`) remains future hardening work (PRD Phase 7 / future scope).

## Phase 5: Logs (REST tail + WebSocket stream)

Phase 5 adds deployment log retention to disk plus server APIs for historical and live streaming.

### Retention model

- Build/deploy logs are written to files under `<data-dir>/logs/<deployment-id>.log` and persisted in `deployments.logs_path`.
- Runtime container logs are streamed from Docker Engine on demand and are not persisted by HostForge in v1.

### API surface

- Historical tail (build log file): `GET /api/deployments/{deployment_id}/logs`
  - Optional query params:
    - `tail_bytes` (default `65536`, capped to prevent unbounded reads)
    - `tail_lines` (optional, trims to ending N lines)
- Live WebSocket stream: `GET /api/deployments/{deployment_id}/logs/live`
  - `?source=build` streams appended file output.
  - `?source=container` streams Docker `ContainerLogs` for the deployment container.
  - Default source prefers container logs for successful deployments, otherwise build logs.

### Security note (pre-Phase 7)

Log APIs/WebSockets are unauthenticated in Phase 5. Do not expose them publicly. Bind HostForge to localhost or protect with a trusted reverse proxy / firewall / SSH tunnel until Phase 7 hardening.

## Phase 6: UI (Vite + React + TypeScript + Tailwind)

Phase 6 adds a browser control plane served by `cmd/server`, with observability and controls over the same backend orchestration used by CLI/webhooks.

### Stack and delivery

- UI source lives in `web/`.
- Stack: **Vite**, **React**, **TypeScript**, **TailwindCSS**.
- Production delivery: build to `web/dist` and run `hostforge-server`; the server serves static assets plus API routes on one origin.
- Development: run Vite dev server and proxy `/api` + WebSocket paths to HostForge server.

### Build and run UI

```bash
# from repo root
npm --prefix web install
npm --prefix web run build
go build -o hostforge-server ./cmd/server
./hostforge-server -data-dir ./.hostforge -listen :8080
```

For local UI iteration:

```bash
# terminal 1
go run ./cmd/server -data-dir ./.hostforge -listen :8080

# terminal 2
npm --prefix web run dev
```

Vite proxy config (`web/vite.config.ts`) forwards:

- `/api/*` → `http://127.0.0.1:8080`
- `/hooks/*` → `http://127.0.0.1:8080`
- WebSocket upgrades on `/api/deployments/{id}/logs/live`

### New API surface for UI

- `GET /api/projects`
- `POST /api/projects` (create project from repo URL/branch/name)
- `GET /api/projects/{id}`
- `DELETE /api/projects/{id}` (removes project, deployments, domains, and stops/removes linked Docker containers; syncs Caddy when domains existed or `HOSTFORGE_SYNC_CADDY` is set)
- `GET /api/projects/{id}/domains`
- `GET /api/projects/{id}/deployments`
- `GET /api/deployments` (global deployment list)
- Existing logs APIs:
  - `GET /api/deployments/{id}/logs`
  - `GET /api/deployments/{id}/logs/live` (WebSocket)
- Control endpoints:
  - `POST /api/projects/{id}/deploy`
  - `POST /api/projects/{id}/restart`
  - `POST /api/projects/{id}/rollback`
  - `POST /api/projects/{id}/stop`

### Wizard and UI behavior

- New project flow supports:
  1. Source step (repo URL, branch default `main`, name suggestion)
  2. Immediate deploy trigger and transition to BUILDING state
  3. Live deployment view with WebSocket logs
  4. Success/failure states with follow-up actions
- Environment-variable configuration is intentionally deferred from Phase 6 and remains future scope.

### UI structure (post-redesign)

The UI follows the brutalist guidelines from `Design1.md` (no rounded corners, borders over shadows, lightness-shift hovers). Code is split as:

- `web/src/components/` — `Shell`, `Sidebar`, `Topbar`, `ThemeToggle`, plus primitives `Panel`, `KpiTile`, `StatusPill`, `Button`, `EmptyState`, `Stepper`, `Terminal`.
- `web/src/pages/` — `DashboardPage`, `ProjectsPage`, `ProjectPage`, `DeploymentPage`, `NewProjectPage`.
- `web/src/theme.ts` — theme bootstrap and persistence.
- `web/src/format.ts` — date/duration/short-hash helpers.

Routes:

- `/` — Overview dashboard (KPI tiles + recent deployments + system panel)
- `/projects` — Project fleet (with All / Running / Failed filters)
- `/projects/new` — New project wizard
- `/projects/:id` — Project header + Controls + Deployment history + Danger zone
- `/projects/:id/deployments/:id` — Deployment metadata + live terminal

### Theming

- Colors are exposed as CSS variables (`--hf-bg`, `--hf-surface`, `--hf-border`, `--hf-primary`, …) defined in `web/src/index.css`, and consumed via Tailwind semantic tokens (`bg-bg`, `bg-surface`, `border-border`, `text-primary`, …) declared in `web/tailwind.config.js`.
- On first load the app reads `prefers-color-scheme` and applies dark or light. The header toggle (`ThemeToggle`) flips themes and persists the choice in `localStorage` (`hf-theme`); once set, the user choice overrides system preference. Without a stored choice, system changes are followed live.
- Both palettes preserve the same component structure: only color vars change. The `* { border-radius: 0 !important; }` rule keeps the brutalist no-radius look in either mode.

### Security note (pre-Phase 7)

Phase 6 management APIs and WebSocket streams remain unauthenticated until Phase 7 hardening. Keep the server private (localhost bind, SSH tunnel, firewall, or trusted reverse proxy).
