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
| `HOSTFORGE_LISTEN` | Server listen address for API, UI, auth, and webhooks (default: `:8080`) |
| `HOSTFORGE_WEBHOOK_BASE_PATH` | Webhook route path (default: `/hooks/github`) |
| `HOSTFORGE_WEBHOOK_MAX_BODY_BYTES` | Max webhook body size in bytes (default: `1048576`) |
| `HOSTFORGE_WEBHOOK_ASYNC` | If `true`, accept webhook deploys with `202` and run in background |
| `HOSTFORGE_WEBHOOK_SECRET` | **Required** for production: GitHub webhook signing secret; server verifies `X-Hub-Signature-256` (HMAC SHA-256 of raw body) |
| `HOSTFORGE_WEBHOOK_RATE_LIMIT_PER_MINUTE` | Per-IP ceiling on webhook POSTs (default: `60`, must be `> 0`) |
| `HOSTFORGE_API_TOKEN` | **Required:** static `Authorization: Bearer` token for management APIs and for `POST /auth/session` (UI login uses this as the password) |
| `HOSTFORGE_SESSION_SECRET` | **Required:** HMAC key for signed UI session cookies (minimum **16** characters) |
| `HOSTFORGE_SESSION_COOKIE_NAME` | Session cookie name (default: `hostforge_session`) |
| `HOSTFORGE_SESSION_TTL_MINUTES` | Session lifetime (default: `720`) |
| `HOSTFORGE_SESSION_COOKIE_SECURE` | If `true`, set `Secure` on session cookies (use behind HTTPS) |
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

### Security (Phase 7)

- **`HOSTFORGE_WEBHOOK_SECRET` is required** at server startup. GitHub must send **`X-Hub-Signature-256`** (`sha256=<hex>`); HostForge rejects missing or mismatched signatures (**`401`**).
- **Rate limiting:** webhook POSTs are capped per client IP using **`HOSTFORGE_WEBHOOK_RATE_LIMIT_PER_MINUTE`** (returns **`429`** when exceeded).
- Management **REST** and **WebSocket** log streams under `/api/...` require either a valid **`Authorization: Bearer <HOSTFORGE_API_TOKEN>`** header or a valid **signed HttpOnly session cookie** (see Phase 7 below).

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

### Security (Phase 7)

Historical and live log endpoints require the same authentication as other management APIs (bearer token or valid UI session cookie). Prefer binding the server to **loopback** and exposing the UI only through **Caddy** or an SSH tunnel (see **Production hardening** below).

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

Vite proxy config (`web/vite.config.ts` and `web/vite.config.js`) forwards:

- `/api/*` → `http://127.0.0.1:8080` (including WebSocket upgrades for `/api/deployments/{id}/logs/live`)
- `/hooks/*` → `http://127.0.0.1:8080`
- `/auth/*` → `http://127.0.0.1:8080` (session cookie login for the UI)

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

### Security (Phase 7)

The UI signs in via **`POST /auth/session`** with header **`Authorization: Bearer <HOSTFORGE_API_TOKEN>`** (same secret as the management API token). On success the server sets an **HttpOnly** session cookie (`HOSTFORGE_SESSION_COOKIE_NAME`). Subsequent **`GET /api/...`** and WebSocket requests send that cookie automatically from the browser.

Automation and the CLI should send **`Authorization: Bearer <HOSTFORGE_API_TOKEN>`** on management routes.

---

## Phase 7: Hardening (authentication, webhooks, install)

### Management API and UI sessions (v1)

- **Bearer token (CLI / scripts):** `Authorization: Bearer <HOSTFORGE_API_TOKEN>` on all `/api/*` routes (including log tail and WebSocket upgrade).
- **Browser UI:** `POST /auth/session` with **`Authorization: Bearer`** (same token) → **signed** session cookie; `GET /auth/session` reports auth state; `DELETE /auth/session` clears the cookie.
- Either credential type satisfies `requireManagementAuth` for REST and WebSockets.

The server refuses to start if **`HOSTFORGE_API_TOKEN`**, **`HOSTFORGE_SESSION_SECRET`** (length ≥ 16), **`HOSTFORGE_WEBHOOK_SECRET`**, or **`HOSTFORGE_WEBHOOK_RATE_LIMIT_PER_MINUTE`** (must be > 0) is missing or invalid, or if session cookie name / TTL are invalid.

### GitHub webhook configuration (Phase 7)

- Webhook **Content type:** `application/json`
- **Secret:** set to the same value as **`HOSTFORGE_WEBHOOK_SECRET`** (GitHub signs the raw body; HostForge verifies **`X-Hub-Signature-256`**).

### Install from source (`scripts/install.sh`)

From a repository clone (requires **Go** on the build machine):

```bash
./scripts/install.sh
```

- Builds **`hostforge`** (`cmd/cli`) and **`hostforge-server`** (`cmd/server`) and installs them under **`PREFIX/bin`** (default **`/usr/local/bin`**). Re-run anytime; binaries are replaced idempotently.
- Optional **systemd** layout (Linux, root):

```bash
sudo ./scripts/install.sh --with-systemd
```

This creates user **`hostforge`**, data dir **`/var/lib/hostforge`**, **`/etc/hostforge/hostforge.env`** from [`scripts/hostforge-server.env.example`](./scripts/hostforge-server.env.example) **only if** `hostforge.env` does not already exist, installs **`/etc/systemd/system/hostforge-server.service`**, runs **`systemctl daemon-reload`** and **`enable`**. Edit secrets in **`/etc/hostforge/hostforge.env`**, then:

```bash
sudo systemctl start hostforge-server
```

Flags: **`--prefix`**, **`--data-dir`**, **`--with-systemd`**, **`--skip-build`** (use existing `./hostforge` binaries in the repo root). If **`docker`** group exists, **`hostforge`** is added to it so the service can talk to Docker Engine.

**Caddy** is not installed by this script; install it separately, open **80/443**, and `reverse_proxy` to HostForge (e.g. `127.0.0.1:8080`) when exposing the UI or TLS-terminated webhooks.

### Secrets: storage, permissions, rotation

| Item | Recommendation |
|------|------------------|
| **`/etc/hostforge/hostforge.env`** | Mode **`0640`**, owner **`root`**, group **`hostforge`** so the service user can read but not write secrets. |
| **Rotation — API token** | Generate a new random token, update **`HOSTFORGE_API_TOKEN`** in the env file, restart **`hostforge-server`**, update any clients/GitHub does not use this for webhooks. |
| **Rotation — session secret** | Changing **`HOSTFORGE_SESSION_SECRET`** invalidates all existing UI sessions; users sign in again. Schedule with API token rotation if compromised. |
| **Rotation — webhook secret** | Update secret in GitHub repo webhook settings and **`HOSTFORGE_WEBHOOK_SECRET`** together, then reload the service. |
| **Backups** | Treat **`hostforge.env`** and **`hostforge.db`** as sensitive; restrict filesystem permissions and backup encryption. |

Never commit real values; keep an **`*.example`** file in version control only.

### Production hardening: firewall and process ownership

**Firewall (typical VPS):**

- Allow **inbound TCP 80** and **443** for **Caddy** (public HTTP/S).
- Bind HostForge to **`127.0.0.1:8080`** (default in the generated env file) so the management API and UI are **not** reachable from the Internet unless you forward them through Caddy or an SSH tunnel.
- If you must bind `:8080` on all interfaces, restrict it with **`ufw`** / **`nftables`** to admin IPs only.

**UFW-style example (adjust interfaces / subnets):**

```bash
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
# Optional: only if API must be WAN-reachable on 8080 (not recommended)
# sudo ufw allow from <ADMIN_IP> to any port 8080 proto tcp
sudo ufw enable
```

**Process ownership:**

- Run **`hostforge-server`** as a dedicated non-login user (**`hostforge`** from `--with-systemd`). It needs read access to **`/etc/hostforge/hostforge.env`**, read/write to **`HOSTFORGE_DATA_DIR`**, and access to the **Docker** socket (e.g. membership in **`docker`** group — understand the security tradeoff of Docker group access).
- Run **Caddy** under its own user per distro packages; it binds **80/443** and proxies to loopback.

### Phase 7 verification checklist

Run on a **fresh VPS** or clean VM after **`./scripts/install.sh --with-systemd`** and editing **`/etc/hostforge/hostforge.env`**:

1. **Start:** `sudo systemctl start hostforge-server` — expect **active** (`systemctl status`).
2. **Negative — API without auth:** `curl -sS -o /dev/null -w "%{http_code}" http://127.0.0.1:8080/api/projects` → expect **`401`**.
3. **Positive — API with bearer:** `curl -sS -H "Authorization: Bearer $HOSTFORGE_API_TOKEN" http://127.0.0.1:8080/api/projects` → **`200`** and JSON list.
4. **Negative — webhook without signature:** `curl -sS -o /dev/null -w "%{http_code}" -X POST http://127.0.0.1:8080/hooks/github -H "Content-Type: application/json" -d '{}'` → **`401`** (missing **`X-Hub-Signature-256`**).
5. **Positive — UI path:** open the server URL via Caddy or `http://127.0.0.1:8080` (dev-style), sign in with the configured API token as password; confirm projects load and logs stream.
6. **Logout:** use UI logout or `curl -X DELETE -c /tmp/hf.txt -b /tmp/hf.txt ...` as appropriate to confirm session clears.

Unauthorized callers must not trigger deploys (no valid GitHub signature) or read management data/logs (no bearer / session).
