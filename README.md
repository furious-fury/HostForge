# HostForge

Self-hosted PaaS: Git → [Nixpacks](https://github.com/railwayapp/nixpacks) → Docker on one machine, with a management **API**, **browser UI**, **GitHub webhooks**, and **Caddy** for public TLS routing.

## Release version

Bump **`internal/version/VERSION`** (one line, semver). Go embeds it via `internal/version`; the Vite UI reads the same file at dev/build time (`web/vite.config.ts`). CLI `hostforge version` and the dashboard **Build version** field stay aligned automatically.

## Production install

From a repository clone on the build host (**Go** required):

```bash
./scripts/install.sh
```

- Builds **`hostforge`** (`cmd/cli`) and **`hostforge-server`** (`cmd/server`) and installs them under **`PREFIX/bin`** (default **`/usr/local/bin`**). Re-run anytime; binaries are replaced idempotently.
- Optional **systemd** layout (Linux, root): `sudo ./scripts/install.sh --with-systemd` — creates user **`hostforge`**, data dir **`/var/lib/hostforge`**, seeds **`/etc/hostforge/hostforge.env`** from [`scripts/hostforge-server.env.example`](./scripts/hostforge-server.env.example) **only if** it does not exist, installs **`/etc/systemd/system/hostforge-server.service`**, runs **`daemon-reload`** and **`enable`**. Edit secrets in **`/etc/hostforge/hostforge.env`**, then `sudo systemctl start hostforge-server`.
- Flags: **`--prefix`**, **`--data-dir`**, **`--with-systemd`**, **`--skip-build`**. If the **`docker`** group exists, **`hostforge`** is added so the service can use the Docker socket.

**Caddy** is not installed by the script; install it separately, open **80/443**, and reverse-proxy to HostForge (e.g. `127.0.0.1:8080`) when exposing the UI or TLS-terminated webhooks. Env reference, secrets, firewall, and a smoke checklist: [**Authentication, installer, and operations**](#authentication-installer-and-operations).

## Quick start (local development)

1. **Toolchain:** **Go 1.22+**, **Git**, **Nixpacks** on `PATH`, **Docker Engine** (deploy builds a container image and runs it).

2. **CLI**

   ```bash
   go build -o hostforge ./cmd/cli
   ```

3. **Server env** — copy [`scripts/hostforge-server.env.example`](./scripts/hostforge-server.env.example) and set **`HOSTFORGE_API_TOKEN`**, **`HOSTFORGE_SESSION_SECRET`** (≥16 characters), **`HOSTFORGE_WEBHOOK_SECRET`**, **`HOSTFORGE_WEBHOOK_RATE_LIMIT_PER_MINUTE`** (must be greater than `0`). Set **`HOSTFORGE_DATA_DIR`** to a writable directory and export the variables (or use your shell’s env-file mechanism).

4. **Web UI assets**

   ```bash
   npm --prefix web install
   npm --prefix web run build
   ```

5. **Run the server**

   ```bash
   go run ./cmd/server -data-dir "$HOSTFORGE_DATA_DIR" -listen "${HOSTFORGE_LISTEN:-:8080}"
   ```

6. **UI hot reload** — in another terminal, `npm --prefix web run dev` (Vite proxies `/api`, `/hooks`, `/auth` to the Go server).

7. **Webhooks from GitHub while on loopback** — use [`scripts/ngrok-dev.sh`](./scripts/ngrok-dev.sh); see [Local public URL (ngrok)](#local-public-url-ngrok-free-tier).

## What’s included

- **CLI (`cmd/cli`):** `deploy`, `domain` add/edit/remove, `caddy sync`, `validate`, `version`; same deploy pipeline as the server.
- **Server (`cmd/server`):** REST + embedded UI (`web/dist`), GitHub **`push`** webhooks, cookie-backed UI login, SQLite persistence, bounded **observability** rows (`deploy_steps`, `http_requests`) with **`/observability`** and per-deployment **Steps**.
- **Caddy:** generated snippet, **`caddy validate`** / **`caddy reload`** when configured; health-gated zero-downtime cutover before switching routes.
- **UI (`web/`):** projects, deployments (REST + WebSocket logs), domains with DNS hints and registrar refresh, dashboard **System** panel (`GET /api/system/status`) plus **Host** KPIs (`GET /api/system/host/snapshot` / `history`), **Settings** page (`GET /api/settings` + POST actions under `/api/settings/actions/…`), TanStack Query on fleet pages. **`GET /api/projects`** defaults to a fast list (no per-domain live registrar lookups); use **`?dns=1`** for full DNS checks in one response. **`GET /api/deployments?limit=N`** uses SQL `LIMIT` and batched container rows. System status checks run **in parallel** with a **5s** cached snapshot. Host metrics are **Linux-only** (in-memory ~30 min ring by default).
- **Marketing / docs site (`site/`):** separate Vite + React static build (landing + prerendered docs, raw `.md` and `llms.txt` for agents). See [`site/README.md`](./site/README.md).

**Operators:** DNS **A/AAAA** must point at the host where **Caddy** serves **80/443**; firewall / cloud SG must allow inbound **80/443**. Residential WAN IPs change—use a VPS, static IP, or DDNS for stable webhooks. Planning docs: `task_list.md`, PRD — this README describes the **current** tree.

## Prerequisites

- **Go** 1.22+, **Git**, **Nixpacks** on `PATH`
- **Docker Engine** for image-based deploys; **`DOCKER_HOST`**, `DOCKER_TLS_VERIFY`, `DOCKER_CERT_PATH` honored by the client
- **Windows:** prefer **WSL2** or Linux for Nixpacks; ensure Docker is reachable (or set `DOCKER_HOST`)

## Build

```bash
go build -o hostforge ./cmd/cli
go build -o hostforge-server ./cmd/server
```

## Usage

```bash
./hostforge deploy [flags] <repo_url>
./hostforge domain add [flags] --domain <hostname> <repo_url>
./hostforge domain remove [flags] (--id <domain_id> | --domain <hostname> <repo_url>)
./hostforge domain edit [flags] --id <domain_id> --domain <new_hostname>
./hostforge caddy sync [flags]
./hostforge validate docker|preflight
./hostforge version
```

- **`<repo_url>`:** HTTPS clone URL (e.g. `https://github.com/org/repo`).
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

**If clone fails:** Check network, URL, and private-repo access (configure Git credentials in your environment as needed).

**If Nixpacks fails:** Run `nixpacks plan .` inside the worktree path printed in logs, or install/upgrade Nixpacks. Ensure sufficient disk and that the stack is supported by Nixpacks.

**Deploy images:** `hostforge deploy` uses `nixpacks build . --name <image>` so Nixpacks emits a Docker image and HostForge runs a container. Image tags use `hostforge/<worktree-slug>:<utc-build-id>`.

### Bun apps and Nixpacks (Node 18 EOL)

Some Bun + frontend repos trigger Nixpacks’ **Node** provider with **`nodejs_18`** in the setup phase. On current **nixpkgs**, Node 18 is removed (`Node.js 18.x has reached End-Of-Life and has been removed`), so the image build fails even though the app runs on **Bun** elsewhere (e.g. Vercel).

**HostForge override:** each project can set **`deploy.runtime`** to **`bun`**. On every deploy, after `git clone`, HostForge writes a **worktree-local `nixpacks.toml`** (not committed to your repo) with **`[variables] NIXPACKS_NODE_VERSION = "20"`**, **`[phases.setup]` `nixPkgs` including `bun` and `nodejs_20`** (not 18), and default **`bun install` / `bun run build` / `bun run start`** when you leave the command fields empty. (We intentionally do **not** set `providers = ["bun"]` — several Nixpacks versions error with **Provider bun not found**.) Deploy logs include a short banner showing the effective runtime and commands.

**Precedence:** your repository may already contain **`nixpacks.toml`**. Nixpacks merges layers according to its own rules; HostForge’s generated file lives in the same directory as the clone. Prefer **repo-owned `nixpacks.toml`** for complex or version-controlled plans; use **HostForge project settings** for a consistent operator default across branches without editing the repo.

**Debugging:** from the printed worktree path, run **`nixpacks plan .`** and inspect the setup/install/build/start phases. Compare with the **`hostforge: ===== generated worktree nixpacks.toml`** section in the deployment build log.

### Stack labels (UI)

After the worktree **`nixpacks.toml`** step on each deploy, HostForge runs **`nixpacks plan . -f json`** and persists **`stack_kind`** (a stable slug such as `node`, `node_vite`, `node_next`, `node_spa`, or `go`) and **`stack_label`** (short human text, e.g. **Node · Vite**) on that **deployment** row. Node apps are refined using **`package.json`** in the clone (Next, Vite, Remix, CRA, …) plus plan phase hints (e.g. **`.next/cache`**). The JSON API exposes them on each deployment and mirrors them onto the project’s **`latest_deployment`** (and top-level **`stack_kind`** / **`stack_label`**) for fleet list views. Rows created before this feature, or deploys where **`plan`** fails (e.g. **`nixpacks`** not on `PATH`), keep empty stack fields until a successful redeploy captures a plan.

Older deployments with empty **`stack_kind`** / **`stack_label`** pick up values on the next successful deploy (the server runs **`nixpacks plan`** during the pipeline). HostForge does **not** ship a bulk backfill in this repository—avoiding checked-in tools that clone repos and write to your DB keeps the default tree safer to publish.

**Stack icons (UI):** optional files under **`web/public/stack-icons/`**. For each stack, the UI tries a small basename list (**`.png` then `.svg`** per name), then **`default`**, then **`node`**, then a built-in SVG glyph. Shipped **`default.svg`** is the generic placeholder; add **`node.png`** as a broad Node/default image if you like. **Aliases:** **`go`** → `golang`, **`node_next`** → `next`, **`node_cra`** → `react`, **`node_vite`** → `vite`, **`node_nuxt`** → `vue`. For **Staticfile** plans ( **`stack_kind` `unknown`** and label **Staticfile**), **`html5`** is tried first. You can still add literal **`{stack_kind}.png`** for any slug (e.g. **`node_remix.png`**). Use square artwork (raster icons are shown at **32×32** CSS pixels; source assets can be larger, e.g. 48×48).

### Operator validation ([`task_list.md`](./task_list.md) — Detailed backlog §1)

Use **[`docs/operator-validation-phase1.md`](./docs/operator-validation-phase1.md)** for staged proof of Docker (**1.1**), public HTTPS + restarts (**1.2**), and zero-downtime cutover (**1.3**). For **1.1** you can also run **`./scripts/operator-validation-phase1.sh`** (full automation) or **`hostforge validate docker`** / **`preflight`** for quick checks. Item **1.1** is recorded **PASS** in the runbook (2026-04-19, WSL2 + Docker Engine); **1.2** / **1.3** need a VPS with real DNS + Caddy—complete the runbook checklists and paste evidence in that file, then tick the matching exit rows in `task_list.md`.

## Caddy and public HTTPS

HostForge writes a **generated Caddyfile fragment** under the data directory and runs **`caddy validate`** / **`caddy reload`** against a **root** Caddyfile you maintain that **imports** that fragment (see [implementation_plan.md](./implementation_plan.md)). **v1 does not use Caddy’s Admin API.**

### Install and host networking

- Install [Caddy](https://caddyserver.com/docs/install) by package or static binary; use a **recent 2.x** release.
- Caddy must be able to bind **80** and **443** on the VPS for automatic HTTPS (Let’s Encrypt). If you bind elsewhere, adjust your root Caddyfile accordingly.
- **DNS:** point each public hostname (`domain add`) at this host with **`A`/`AAAA`** to the server’s public IP before TLS issuance will succeed.

### DNS: what HostForge cannot do (and what to paste in your DNS panel)

HostForge **does not** log in to Hostinger, Cloudflare, Route53, etc. There is **no one-liner** that creates DNS records for an arbitrary customer domain: that always happens in **their** DNS manager (or via a **future** optional integration if you add provider APIs and stored credentials).

**What you can do today:** give the operator (or the app owner) **exact values** to enter manually.

1. **On the HostForge / Caddy server**, get the public IPv4 you want the world to use (must match where Caddy listens on **80** and **443**):

   ```bash
   curl -4s ifconfig.me/ip
   ```

   Write that down as **`SERVER_IP`**. (If you use IPv6-only or dual-stack, also collect your provider’s public IPv6 for **`AAAA`**.)

2. **In the DNS zone for the hostname** (e.g. `mrfury.dev` at Hostinger, or `app.customer.com` at the customer’s registrar), create or update records so the name resolves to **`SERVER_IP`**:

   | Record | Name / Host (typical) | Points to / Value |
   |--------|------------------------|-------------------|
   | **A** | `@` or blank (apex) | `SERVER_IP` |
   | **A** | `www` (optional) | `SERVER_IP` if you want `www.` to work |
   | **AAAA** | `@` (optional) | Your server’s public IPv6 |

   For a **subdomain only** (e.g. `app.example.com`), add an **A** record with host **`app`** (not `@`) → `SERVER_IP`.

3. **Remove** conflicting records: delete or replace old **A** records that still point to a **parking / default host** page (common on shared hosting). Until `dig` shows your **`SERVER_IP`**, the browser will keep hitting the old host, not Caddy.

4. **Verify from your laptop** (after a few minutes, sometimes up to 48h for slow TTLs):

   ```bash
   dig +short yourhostname.example A
   ```

   The answer must be **`SERVER_IP`**. Then try `curl -I https://yourhostname.example/`.

5. **Firewall / cloud security group:** allow inbound **TCP 80** and **443** to this host so Let’s Encrypt (HTTP-01) and browsers can reach Caddy.

**Summary for a “DNS handoff” blurb you can send to a user:** Point **A** `@` (and **A** `www` if needed) for their domain to your server’s public IPv4 (`SERVER_IP` from step 1), remove any old parking **A** records, wait for DNS to propagate, then HTTPS will work once that hostname is registered with `hostforge domain add` and Caddy has synced.

The **UI** (project page) and **`hostforge domain add` / `edit`** output print **suggested DNS rows** using auto-detected public IP (or `HOSTFORGE_DNS_SERVER_IPV4` / `HOSTFORGE_DNS_SERVER_IPV6` overrides). Detection can fail behind strict egress firewalls—set overrides in that case.

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
| `HOSTFORGE_ENV_ENCRYPTION_KEY` | **Optional but required for UI env management:** standard **base64** encoding of **32 raw bytes** (AES-256), e.g. `openssl rand -base64 32`. Used to encrypt per-project environment variable **values** at rest in SQLite. If unset, **`GET/POST/PUT/DELETE /api/projects/:id/env`** returns **`503`** with `env_encryption_key_missing`, and deploys **skip** injecting stored project env (deploy still works). **If you lose this key, stored values cannot be decrypted** — rotate by re-entering vars in the UI after setting a new key. |
| `HOSTFORGE_LOGS_DIR` | Optional override for deployment build logs directory (default: `<data-dir>/logs`) |
| `HOSTFORGE_DNS_SERVER_IPV4` | Optional: fixed public IPv4 shown in DNS guidance (skips auto-detect when set) |
| `HOSTFORGE_DNS_SERVER_IPV6` | Optional: fixed public IPv6 for AAAA suggestions |
| `HOSTFORGE_DNS_DETECT_URL` | URL returning plain-text public IPv4 (default: `https://api.ipify.org`) |
| `HOSTFORGE_DNS_DETECT_IPV6_URL` | URL returning plain-text public IPv6 (default: `https://api64.ipify.org`; may return v4-only on some networks) |
| `HOSTFORGE_DNS_DETECT_TIMEOUT_MS` | Timeout for outbound IP discovery (default: `2500`) |
| `HOSTFORGE_DOMAIN_SYNC_AFTER_MUTATE` | If `true` (default), run Caddy sync after domain add/edit/delete API or CLI when `HOSTFORGE_CADDY_ROOT_CONFIG` is set |
| `HOSTFORGE_CADDY_CERT_POLL_INTERVAL_SEC` | **Optional:** if `>0`, `hostforge-server` periodically refreshes per-domain **`last_cert_message`** / **`cert_checked_at`** from a read-only Caddy admin probe plus an optional on-disk leaf cert scan (default: `0` = off) |
| `HOSTFORGE_CADDY_ADMIN` | Caddy admin API base URL for read-only `GET /config/` probes (default: `http://127.0.0.1:2019`) |
| `HOSTFORGE_CADDY_STORAGE_ROOT` | **Optional:** Caddy on-disk storage root (e.g. `~/.local/share/caddy` or `/var/lib/caddy`) so the poll can read managed `*.crt` leaf metadata under `certificates/` — improves summaries when set |

### HTTPS / ACME

TLS is handled by **Caddy automatic HTTPS** (typically Let’s Encrypt). Certificate storage and renewal are **Caddy’s responsibility** on disk (see upstream Caddy docs for data dirs and [staging CA](https://caddyserver.com/docs/caddyfile/options#acme-ca) for testing). HostForge records domain `ssl_status` in SQLite from **validate/reload** success or failure, not by parsing ACME events.

**Optional cert hints (detailed backlog §2.1):** when `HOSTFORGE_CADDY_CERT_POLL_INTERVAL_SEC` is set, the server also stores operator-facing **`last_cert_message`** (e.g. leaf expiry from Caddy’s cert files) and **`cert_checked_at`**. This does **not** replace Caddy’s ACME engine or `ssl_status` (route/snippet sync); it is a best-effort mirror for the dashboard. See [docs/caddy-cert-poll.md](./docs/caddy-cert-poll.md).

### Per-project environment variables

- **Runtime only:** variables are passed to **`docker run`** as extra `KEY=value` pairs (with `PORT` always set by HostForge). They are **not** injected into the Nixpacks build environment; build-time secrets (e.g. some `NEXT_PUBLIC_*` patterns) are out of scope for this feature.
- **API:** `GET /api/projects/:id/env` lists `{ id, key, value_last4, updated_at }` (no plaintext). `POST` upserts by key; `PUT /api/projects/:id/env/:envID` replaces value; `DELETE` removes a row. Keys must match `^[A-Z][A-Z0-9_]*$` after normalization; **`PORT`** is rejected; max **100** vars per project; max **8 KiB** per value.
- **UI:** New Project wizard and Project page editors require the encryption key to be set; otherwise the UI explains the `503` / missing-key state.
- **Redeploy:** after changing env vars in the UI, **redeploy** (or restart with container recreate) so a new process reads the updated map.

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

## GitHub webhooks and server

The server accepts GitHub `push` webhooks and runs the same deployment pipeline as `hostforge deploy`.

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

### Authentication (webhooks)

- **`HOSTFORGE_WEBHOOK_SECRET` is required** at server startup. GitHub must send **`X-Hub-Signature-256`** (`sha256=<hex>`); HostForge rejects missing or mismatched signatures (**`401`**).
- **Rate limiting:** webhook POSTs are capped per client IP using **`HOSTFORGE_WEBHOOK_RATE_LIMIT_PER_MINUTE`** (returns **`429`** when exceeded).
- Management **REST** and **WebSocket** log streams under `/api/...` require either a valid **`Authorization: Bearer <HOSTFORGE_API_TOKEN>`** header or a valid **signed HttpOnly session cookie** (see [Authentication, installer, and operations](#authentication-installer-and-operations)).

## Deployment logs (REST tail + WebSocket stream)

Build/deploy logs are retained on disk with APIs for historical tail and live streaming.

### Retention model

- Build/deploy logs are written to files under `<data-dir>/logs/<deployment-id>.log` and persisted in `deployments.logs_path`.
- Runtime container logs are streamed from Docker Engine on demand and are not persisted by HostForge in v1.

### API surface

- Historical tail (build log file): `GET /api/deployments/{deployment_id}/logs`
  - Optional query params:
    - `tail_bytes` (default `65536`, capped to prevent unbounded reads)
    - `tail_lines` (optional, trims to ending N lines)
  - Response header **`X-Log-EOF-Offset`**: exclusive byte offset at end of file when the tail was read (used by the UI to resume the live WebSocket without duplicating bytes).
  - Query **`eof_meta=1`** (or `eof_meta=true`): response body is JSON `{"eof":<int>,"text":"<tail>"}` with the same tail rules—use this when a proxy strips `X-Log-EOF-Offset` (the web UI always requests `eof_meta=1`).
- Live WebSocket stream: `GET /api/deployments/{deployment_id}/logs/live`
  - `?source=build` streams appended file output.
  - `?source=container` streams Docker `ContainerLogs` for the deployment container.
  - Default source prefers container logs for successful deployments, otherwise build logs.
  - **`format=json`** (default): each WebSocket **text** frame is one JSON object:
    - `{"t":"hello",...}` — protocol version, source, whether byte `cursor` resume is supported, and for build logs the current **`eof`** file size.
    - `{"t":"chunk","end":<int>,"d":"<text>"}` — build log bytes; **`end`** is the exclusive byte offset in the log file after `d`.
    - `{"t":"chunk","seq":<int>,"d":"<text>"}` — container log chunks (best-effort; resume is not byte-addressable like files).
    - `{"t":"heartbeat","seq":<int>}` — application-level keepalive so **idle** streams still carry data on paths that do not treat WebSocket **Ping** control frames as activity.
    - `{"t":"resync","reason":"truncated|rotated",...}` — build log file shrank or rotated; clients should reset local state as needed.
    - `{"t":"end","reason":...}` or `{"t":"error","code":...}` — terminal events.
  - **`?cursor=<bytes>`** (build only): resume after the last received **`chunk.end`** (or after **`X-Log-EOF-Offset`** from the HTTP tail).
  - **`format=raw`**: legacy plain-text stream (same framing as pre-JSON clients); use for `curl`/scripts.
  - The server still sends **periodic WebSocket pings**; the UI **reconnects** while a deployment is still `QUEUED` / `BUILDING` if the connection drops.

### Reverse proxies and long-lived log sockets

- Ensure **`Upgrade`** and **`Connection`** headers are passed for `/api/deployments/*/logs/live`, and set **idle / read timeouts** above your longest quiet build phase (or rely on the server’s **pings** + **JSON heartbeats** as activity).
- Some proxies mishandle WebSocket control frames; the JSON **`heartbeat`** exists so the TCP stream still sees periodic application traffic.
- **WSL2 / Windows browser → Linux**: extra hops can reset idle connections; if logs flap between live and reconnecting, compare the same UI against **direct** `http://127.0.0.1:8080` (production-style) vs **Vite dev proxy** to see which layer drops first.
- Server logs: **`deployment log ws opened`** / **`deployment log ws session ended`** / **`deployment log ws ping failed`** (structured `slog`) help attribute disconnects.

### Authentication (logs)

Historical and live log endpoints require the same authentication as other management APIs (bearer token or valid UI session cookie). Prefer binding the server to **loopback** and exposing the UI only through **Caddy** or an SSH tunnel (see **Production hardening** below).

## Web control plane (UI)

Browser UI served by `cmd/server` (static `web/dist` or Vite dev server), sharing the same orchestration as the CLI and webhooks.

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

### Local public URL (ngrok, free tier)

When HostForge runs on **loopback** (e.g. `127.0.0.1:8080`), the internet cannot reach it for **GitHub webhooks** or sharing the UI without opening home-router ports. Use **ngrok** to get an **`https://…ngrok-free.app`** URL that forwards to the same port.

1. Install [ngrok](https://ngrok.com/download) (Windows, macOS, or Linux/WSL).
2. One-time auth: `ngrok config add-authtoken <token>` (token from the [ngrok dashboard](https://dashboard.ngrok.com/get-started/your-authtoken)).
3. Start `go run ./cmd/server` (with env) so it listens on the port in `HOSTFORGE_LISTEN` (default `127.0.0.1:8080`).
4. In another terminal, from the repo root:

```bash
chmod +x ./scripts/ngrok-dev.sh
./scripts/ngrok-dev.sh
```

The ngrok web UI (link printed in the terminal) shows the public URL. Use **`https://<subdomain>.ngrok-free.app/hooks/github`** as the GitHub webhook **Payload URL** (same path as local). **Free tier:** the hostname changes whenever the tunnel process restarts unless you use a paid reserved domain.

Optional: `NGROK_REGION=us|eu|ap|au|in|jp|sa` before the script to pick an edge region.

Vite proxy config (`web/vite.config.ts` and `web/vite.config.js`) forwards:

- `/api/*` → `http://127.0.0.1:8080` (including WebSocket upgrades for `/api/deployments/{id}/logs/live`)
- `/hooks/*` → `http://127.0.0.1:8080`
- `/auth/*` → `http://127.0.0.1:8080` (session cookie login for the UI)

### New API surface for UI

- `GET /api/projects`
- `POST /api/projects` (create project from repo URL/branch/name; optional `deploy`: `{ "runtime": "auto"|"bun", "install_cmd", "build_cmd", "start_cmd" }`)
- `GET /api/projects/{id}` (includes `deploy`, `domains`, and `dns_guidance` when domains exist)
- `PATCH /api/projects/{id}` (body: `{ "deploy": { "runtime", "install_cmd", "build_cmd", "start_cmd" } }` — updates persisted Nixpacks overrides)
- `DELETE /api/projects/{id}` (removes project, deployments, domains, and stops/removes linked Docker containers; syncs Caddy when domains existed or `HOSTFORGE_SYNC_CADDY` is set)
- `GET /api/projects/{id}/domains` (includes `dns_guidance` for all hostnames on the project)
- `POST /api/projects/{id}/domains` (body: `{"domain_name":"app.example.com"}`; returns `domain`, `dns_guidance`, optional `caddy_sync`)
- `PATCH /api/projects/{id}/domains/{domain_id}` (rename hostname; returns `domain`, `dns_guidance`, optional `caddy_sync`)
- `DELETE /api/projects/{id}/domains/{domain_id}` (optional `caddy_sync` in response)
- `GET /api/projects/{id}/deployments`
- `GET /api/deployments` (global deployment list)
- Existing logs APIs:
  - `GET /api/deployments/{id}/logs` (optional `X-Log-EOF-Offset` response header)
  - `GET /api/deployments/{id}/logs/live` (WebSocket; **`format=json`** default, JSON-framed chunks + **`?cursor=`** resume for build logs)
- Control endpoints:
  - `POST /api/projects/{id}/deploy`
  - `POST /api/projects/{id}/restart`
  - `POST /api/projects/{id}/rollback`
  - `POST /api/projects/{id}/stop`

### Wizard and UI behavior

- **Domains:** each project page includes **Domains** management (add / edit / remove hostnames) plus **copyable DNS hints** derived from the same guidance as the API. Caddy reload after changes follows `HOSTFORGE_DOMAIN_SYNC_AFTER_MUTATE` and requires `HOSTFORGE_CADDY_ROOT_CONFIG` when you want automatic sync.

- New project flow supports:
  1. Source step (repo URL, branch default `main`, name suggestion, optional **Bun / Nixpacks** runtime + install/build/start overrides)
  2. Immediate deploy trigger and transition to BUILDING state
  3. Live deployment view with WebSocket logs
  4. Success/failure states with follow-up actions
- Project page includes **Build & runtime (Nixpacks)** to edit the same `deploy` fields and save via `PATCH` (redeploy to rebuild with the new worktree `nixpacks.toml`).
- Per-project environment-variable editing in the UI remains future scope; use the API/CLI patterns you have today.

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
- Theme is **light** or **dark** only, stored in **`localStorage`** as **`hf-prefs`** (field **`theme`**; legacy **`hf-theme`** is still read if **`hf-prefs`** is absent). Any stored **`system`** value is treated as **dark**. An inline script in **`web/index.html`** sets **`data-theme`** and the **`dark`** class before React paints. The header **`ThemeToggle`** cycles light/dark; when supported, **`document.startViewTransition`** wraps the DOM update (see **`web/src/components/Shell.tsx`**).
- Both palettes preserve the same component structure: only color vars change. The `* { border-radius: 0 !important; }` rule keeps the brutalist no-radius look in either mode.

### Authentication (UI)

The UI signs in via **`POST /auth/session`** with header **`Authorization: Bearer <HOSTFORGE_API_TOKEN>`** (same secret as the management API token). On success the server sets an **HttpOnly** session cookie (`HOSTFORGE_SESSION_COOKIE_NAME`). Subsequent **`GET /api/...`** and WebSocket requests send that cookie automatically from the browser.

Automation and the CLI should send **`Authorization: Bearer <HOSTFORGE_API_TOKEN>`** on management routes.

---

## Authentication, installer, and operations

### Management API and UI sessions (v1)

- **Bearer token (CLI / scripts):** `Authorization: Bearer <HOSTFORGE_API_TOKEN>` on all `/api/*` routes (including log tail and WebSocket upgrade).
- **Browser UI:** `POST /auth/session` with **`Authorization: Bearer`** (same token) → **signed** session cookie; `GET /auth/session` reports auth state; `DELETE /auth/session` clears the cookie.
- Either credential type satisfies `requireManagementAuth` for REST and WebSockets.

**Request correlation:** management routes, webhooks, and `/auth/session` run behind a small middleware that assigns a **`request_id`** (from `X-Request-ID` when present, else random) stored on `context` and attached to structured logs. GitHub webhooks also prefer **`X-GitHub-Delivery`** as the correlation id when set. Each HTTP request emits an **`http_request`** log line with `request_id`, `method`, `path`, `status`, and `duration_ms`.

**Stable API `error` codes (JSON):** failure responses use a **snake_case** string in the `error` field (not raw exception text). Deploy / restart / rollback / domain-mutate paths use `internal/services` **coded errors** so the innermost code wins (e.g. `clone_failed`, `health_check_failed`, `caddy_sync_failed`, `docker_unavailable`). Domain validation returns `domain_name_empty`, `domain_name_too_long`, `domain_name_invalid`. Webhook synchronous failures return the same deploy codes as the UI. For full behavior, see tests in `internal/services`, `internal/dnsops`, and `internal/redact`.

**System status (`GET /api/system/status`):** each check may include **`error_code`** (`docker_unreachable`, `caddy_validate_failed`, `webhook_probe_build_failed`, `webhook_route_unreachable`, …) with a short, non-sensitive **`detail`** string for the dashboard.

**Host metrics (Linux only, management auth):** the server samples **`/proc`** / **`/sys`** on a fixed interval (default **5s**) into an in-memory ring (default **360** samples ≈ **30 min**). **`GET /api/system/host/snapshot`** returns the latest sample JSON (`supported: false`, `error_code: unsupported_os` on non-Linux builds); responses are cached for **1s**. **`GET /api/system/host/history?points=N`** returns oldest-first samples ( **`points` capped at 720**). Optional env: **`HOSTFORGE_HOSTMETRICS_INTERVAL_MS`**, **`HOSTFORGE_HOSTMETRICS_CAPACITY`**, **`HOSTFORGE_HOSTMETRICS_NET_INCLUDE`**, **`HOSTFORGE_HOSTMETRICS_NET_EXCLUDE`**, **`HOSTFORGE_HOSTMETRICS_DISK_INCLUDE`** (regex on mount paths).

**Observability UI (SQLite samples):** the management UI includes **`/observability`** plus a **Steps** tab on each deployment. The server persists bounded rows in **`deploy_steps`** (clone / nixpacks / container / health / caddy / `deploy_total`, plus optional **`cert_poll`**) and **`http_requests`** (method, path, status, duration, `request_id`). Authenticated JSON:

- `GET /api/observability/summary` — last **24h** aggregates (HTTP counts, error rate inputs, deploy counts, p50/p95 durations) plus embedded **system** snapshot (same shape as `GET /api/system/status`).
- `GET /api/observability/requests?limit=` — recent HTTP rows.
- `GET /api/observability/deploy-steps?limit=` — recent deploy step rows (joins project name when `project_id` is set).
- `GET /api/deployments/{id}/steps?limit=` — steps for one deployment.

Retention is row-cap based (~5000 per table, oldest trimmed on insert). This is **not** a full log sink — use **`journalctl`** for raw slog lines.

The server refuses to start if **`HOSTFORGE_API_TOKEN`**, **`HOSTFORGE_SESSION_SECRET`** (length ≥ 16), **`HOSTFORGE_WEBHOOK_SECRET`**, or **`HOSTFORGE_WEBHOOK_RATE_LIMIT_PER_MINUTE`** (must be > 0) is missing or invalid, or if session cookie name / TTL are invalid.

### GitHub webhook configuration

- Webhook **Content type:** `application/json`
- **Secret:** set to the same value as **`HOSTFORGE_WEBHOOK_SECRET`** (GitHub signs the raw body; HostForge verifies **`X-Hub-Signature-256`**).

### Installer details

The **`scripts/install.sh`** flow is summarized under [Production install](#production-install) at the top of this file (`--prefix`, `--data-dir`, `--with-systemd`, `--skip-build`, `docker` group membership).

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

### Post-install verification checklist

Run on a **fresh VPS** or clean VM after **`./scripts/install.sh --with-systemd`** and editing **`/etc/hostforge/hostforge.env`**:

1. **Start:** `sudo systemctl start hostforge-server` — expect **active** (`systemctl status`).
2. **Negative — API without auth:** `curl -sS -o /dev/null -w "%{http_code}" http://127.0.0.1:8080/api/projects` → expect **`401`**.
3. **Positive — API with bearer:** `curl -sS -H "Authorization: Bearer $HOSTFORGE_API_TOKEN" http://127.0.0.1:8080/api/projects` → **`200`** and JSON list.
4. **Negative — webhook without signature:** `curl -sS -o /dev/null -w "%{http_code}" -X POST http://127.0.0.1:8080/hooks/github -H "Content-Type: application/json" -d '{}'` → **`401`** (missing **`X-Hub-Signature-256`**).
5. **Positive — UI path:** open the server URL via Caddy or `http://127.0.0.1:8080` (dev-style), sign in with the configured API token as password; confirm projects load and logs stream.
6. **Logout:** use UI logout or `curl -X DELETE -c /tmp/hf.txt -b /tmp/hf.txt ...` as appropriate to confirm session clears.

Unauthorized callers must not trigger deploys (no valid GitHub signature) or read management data/logs (no bearer / session).
