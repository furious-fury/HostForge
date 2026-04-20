# Operator validation — Detailed backlog §1

This runbook implements **[`task_list.md`](../task_list.md) → Detailed backlog → 1. Operator validation and exit criteria**: **1.1** (Docker), **1.2** (HTTPS + restarts), **1.3** (zero-downtime cutover), and includes a **Phase 8 production-proof template** for launch-gate validation.

Use a **staging VPS** (or equivalent) for **1.2** and **1.3**. **1.1** can be run on any host with Docker Engine + Nixpacks + Git + network access to the sample repo.

### Automation (code in-repo)

- **`hostforge validate docker`** — pings Docker Engine with the same client settings as `deploy` (`DOCKER_HOST`, etc.). Exit **0** if the daemon is reachable.
- **`hostforge validate preflight`** — `validate docker` plus **`git`** and **`nixpacks`** on `PATH`.
- **`scripts/operator-validation-phase1.sh`** — end-to-end **§1.1**: preflight, golden-path `deploy -host-port 0`, HTTP **200**, `unless-stopped`, survives `docker restart`, then `docker stop` + `docker rm` and confirms the port is closed. From repo root: `./scripts/operator-validation-phase1.sh` (optional: `HOSTFORGE_BIN=./hostforge`, `REPO_URL=…`, `KEEP_HF_DATA=1`).

| Item | Description | Status |
|------|-------------|--------|
| **1.1** | Phase 1 exit — Docker runtime proof | **PASS** (see [§1.1 execution record](#11-execution-record-2026-04-19-wsl2)) |
| **1.2** | Phase 3 exit — operator HTTPS smoke | **Pending** (requires real DNS + VPS; follow [§1.2](#12-phase-3-exit--https--restart-resilience)) |
| **1.3** | Cutover exit — zero-downtime redeploy | **Pending** (requires Caddy + domain; follow [§1.3](#13-cutover-exit--zero-downtime-redeploy)) |

---

## Prerequisites (all items)

- **Go** 1.22+, **Nixpacks** on `PATH`, **Docker Engine** reachable (`docker info` OK).
- Built CLI: `go build -o hostforge ./cmd/cli` from repo root.
- Optional isolated data dir: `export HF_DATA=/tmp/hostforge-operator-phase1` (use `HOSTFORGE_DATA_DIR` or `-data-dir`).

### Preflight

```bash
docker info >/dev/null && echo "docker: ok"
command -v nixpacks >/dev/null && nixpacks --version
command -v caddy >/dev/null && caddy version   # needed for 1.2 / 1.3 on the VPS
```

---

## 1.1 Phase 1 exit — Docker runtime proof

**Goal:** Container from `hostforge deploy` is reachable on published `host_port`; **restart policy** is `unless-stopped`; **stop + remove** releases the port.

### Steps

1. **Deploy** (golden-path sample; ephemeral host port):

   ```bash
   rm -rf "$HF_DATA" && mkdir -p "$HF_DATA"
   HOSTFORGE_DATA_DIR="$HF_DATA" ./hostforge deploy -host-port 0 \
     https://github.com/heroku/node-js-getting-started
   ```

   Capture from stdout: `container_id`, `host_port`, `url`.

2. **Reachability** (expect HTTP **200** on `/` for this sample):

   ```bash
   curl -sS -o /dev/null -w "%{http_code}\n" "http://127.0.0.1:<host_port>/"
   ```

3. **Restart policy** (must print `unless-stopped` — matches [`internal/docker/runtime.go`](../internal/docker/runtime.go)):

   ```bash
   docker inspect --format '{{.HostConfig.RestartPolicy.Name}}' <container_id>
   ```

4. **Survives `docker restart`** (process comes back; same host port mapping):

   ```bash
   docker restart <container_id>
   sleep 2
   curl -sS -o /dev/null -w "%{http_code}\n" "http://127.0.0.1:<host_port>/"
   ```

5. **Stop and remove** (HostForge does not ship a `hostforge container rm` today; use Docker CLI for this exit, or delete the project via management API which stops/removes containers — see README / server routes):

   ```bash
   docker stop <container_id>
   docker rm <container_id>
   curl -sS --connect-timeout 2 "http://127.0.0.1:<host_port>/" || true   # expect connection failure
   ```

### 1.1 execution record (2026-04-19, WSL2)

| Step | Result |
|------|--------|
| Deploy | `HOSTFORGE_DATA_DIR=/tmp/hostforge-phase1-verify` `go run ./cmd/cli deploy -host-port 0` → **exit 0** |
| IDs / port | `deployment_id=973743266df3f706638b320923f96b52`, `container_id=4d2085116b76…`, `host_port=40473`, `url=http://127.0.0.1:40473` |
| HTTP | `curl` → **200** |
| Restart policy | `docker inspect …` → **`unless-stopped`** |
| After `docker restart` | `curl` → **200** |
| After `docker stop` + `docker rm` | `curl` → **connection refused** (port released) |

**Conclusion:** Phase 1 Docker exit criteria satisfied on this host.

---

## 1.2 Phase 3 exit — HTTPS + restart resilience

**Goal:** Real FQDN over **HTTPS**; survives **HostForge** + **Caddy** restarts (order per [README](../README.md)).

### Staging setup

1. Point **DNS** `A`/`AAAA` for your hostname at the VPS public IP (see README “DNS: what HostForge cannot do”).
2. Install Caddy; prepare **root Caddyfile** that `import`s `HOSTFORGE_CADDY_GENERATED_PATH` (see README Phase 3).
3. Set `HOSTFORGE_CADDY_ROOT_CONFIG` and deploy HostForge server (or use CLI-only with `caddy sync` after deploy).

### Steps

1. Register domain:

   ```bash
   HOSTFORGE_DATA_DIR="$HF_DATA" ./hostforge domain add --domain app.example.com \
     https://github.com/heroku/node-js-getting-started
   ```

2. Deploy with sync (adjust flags to match your Caddy env):

   ```bash
   HOSTFORGE_DATA_DIR="$HF_DATA" ./hostforge deploy -sync-caddy \
     https://github.com/heroku/node-js-getting-started
   ```

3. **External HTTPS check** (from laptop or `curl` on another machine):

   ```bash
   curl -I "https://app.example.com/"
   ```

   Expect **2xx** and a valid certificate chain in the browser.

4. **Restarts** (example with systemd; adapt to your unit names):

   ```bash
   sudo systemctl restart hostforge-server   # or your HostForge unit
   sudo systemctl restart caddy
   ```

   Re-run `curl -I https://app.example.com/` — expect success without hand-editing the generated snippet.

### Evidence checklist

- [ ] `dig +short app.example.com A` matches VPS
- [ ] Browser shows padlock / valid chain
- [ ] After HostForge + Caddy restart, site still loads
- [ ] Note any ACME rate-limit or DNS delay issues observed

**Operator:** When complete, set **1.2** status to **PASS** in the table at the top and paste short evidence (timestamps + commands) below.

#### 1.2 evidence (paste here)

```
(date / operator / environment)
```

---

## 1.3 Cutover exit — zero-downtime redeploy

**Goal:** Traffic through **Caddy** stays healthy during redeploy; **failed** candidate leaves **prior** deployment serving.

### Steps

1. Ensure **1.2** works (hostname routes to current container).
2. **Terminal A** — load generator (hits **public** URL, not raw host port):

   ```bash
   while true; do curl -sS -o /dev/null -w "%{http_code}\n" "https://app.example.com/" || echo ERR; sleep 0.2; done | tee /tmp/hf-cutover-load.log
   ```

3. **Terminal B** — good redeploy (change app response or bump image; same repo):

   ```bash
   HOSTFORGE_DATA_DIR="$HF_DATA" ./hostforge deploy -sync-caddy \
     https://github.com/heroku/node-js-getting-started
   ```

   Observe load log: expect **no sustained connection failures** during cutover; response may switch after health passes.

4. **Failed candidate** — deploy a revision that **fails health** (e.g. wrong `--container-port` vs app listen port, or break `/healthz` if you add one):

   ```bash
   HOSTFORGE_DATA_DIR="$HF_DATA" ./hostforge deploy -sync-caddy -container-port 9999 \
     https://github.com/heroku/node-js-getting-started
   ```

   Expect deploy **FAILED** in DB/UI; **Caddy** should still route to the **last SUCCESS** container; Terminal A should keep returning **2xx**.

### Evidence checklist

- [ ] Good redeploy: no “upstream refused” window in load log
- [ ] Failed deploy: prior container still serves; deployment row shows `FAILED` for candidate

**Operator:** When complete, set **1.3** status to **PASS** and paste excerpts from `/tmp/hf-cutover-load.log` + deployment IDs.

#### 1.3 evidence (paste here)

```
(paste)
```

---

## Related product checkboxes

After **1.1**, mark **Phase 1 exit** in [`task_list.md`](../task_list.md).

After **1.2**, mark **Phase 3 exit (operator smoke test)**.

After **1.3**, mark **Cutover exit (manual smoke)** under Orchestration.

---

## Phase 8 production proof template (launch gate)

Use this section after 1.2/1.3 work is stable to execute the full **Phase 8** checklist from [`task_list.md`](../task_list.md). Keep commands and evidence in one place so release readiness is auditable.

### 8.0 Operator context block (fill first)

```bash
export HF_SERVER_URL="https://hf-admin.example.com"
export HF_PUBLIC_APP_DOMAIN="app.example.com"
export HF_DATA="/var/lib/hostforge"
export HF_TOKEN="<admin-api-token>"
export VPS_HOST="<user@vps-ip-or-hostname>"
```

Record:

- Operator:
- Date/time window:
- VPS provider + size:
- OS image:
- HostForge version (`hostforge version`):
- Caddy version (`caddy version`):
- Docker version (`docker --version`):

### 8.1 Real VPS deployment proof

1. **Install/runtime sanity**:

   ```bash
   ssh "$VPS_HOST" "docker info >/dev/null && echo docker-ok"
   ssh "$VPS_HOST" "caddy version"
   ssh "$VPS_HOST" "hostforge version"
   ssh "$VPS_HOST" "systemctl is-active hostforge-server || true"
   ssh "$VPS_HOST" "systemctl is-active caddy || true"
   ```

2. **Expected results**:
   - `docker-ok`
   - Caddy version string
   - HostForge version string
   - service state `active` for long-running mode

Evidence block:

```text
8.1 evidence:
- command:
- result:
- timestamp:
```

### 8.2 Real domain + HTTPS proof

1. **DNS check**:

   ```bash
   dig +short "$HF_PUBLIC_APP_DOMAIN" A
   dig +short "$HF_PUBLIC_APP_DOMAIN" AAAA
   ```

2. **Domain register + route sync** (from host with HostForge CLI/config):

   ```bash
   HOSTFORGE_DATA_DIR="$HF_DATA" hostforge domain add --domain "$HF_PUBLIC_APP_DOMAIN" \
     https://github.com/heroku/node-js-getting-started
   HOSTFORGE_DATA_DIR="$HF_DATA" hostforge caddy sync
   ```

3. **HTTPS validation**:

   ```bash
   curl -sSI "https://$HF_PUBLIC_APP_DOMAIN/" | sed -n '1p;/^server:/Ip'
   ```

4. **Restart resilience**:

   ```bash
   ssh "$VPS_HOST" "sudo systemctl restart hostforge-server && sudo systemctl restart caddy"
   curl -sSI "https://$HF_PUBLIC_APP_DOMAIN/" | sed -n '1p;/^server:/Ip'
   ```

Expected results:

- DNS points to VPS IP.
- HTTPS responds with `HTTP/1.1 200` or `HTTP/2 200`.
- Route survives service restarts without manual Caddy edits.

### 8.3 Golden deploy matrix

Run one deploy per stack and capture deployment IDs + external URL checks.

Suggested matrix:

- Node/Express
- Nuxt SSR
- Static frontend
- Go API

Template per stack:

```bash
HOSTFORGE_DATA_DIR="$HF_DATA" hostforge deploy -sync-caddy <repo-url>
curl -sS -o /dev/null -w "%{http_code}\n" "https://$HF_PUBLIC_APP_DOMAIN/"
```

Expected results:

- Deploy command exits 0.
- Health/cutover succeeds.
- External HTTPS check returns `200`.

Evidence table:

```text
stack | repo | deployment_id | result
----- | ---- | ------------- | ------
node  |      |               | PASS/FAIL
nuxt  |      |               | PASS/FAIL
static|      |               | PASS/FAIL
go    |      |               | PASS/FAIL
```

### 8.4 Zero-downtime proof

1. **Load loop**:

   ```bash
   while true; do
     code="$(curl -sS -o /dev/null -w "%{http_code}" "https://$HF_PUBLIC_APP_DOMAIN/" || echo ERR)"
     printf "%s %s\n" "$(date -Iseconds)" "$code"
     sleep 0.2
   done | tee /tmp/hf-phase8-cutover.log
   ```

2. **Deploy v1 then v2** while loop runs:

   ```bash
   HOSTFORGE_DATA_DIR="$HF_DATA" hostforge deploy -sync-caddy <repo-v1>
   HOSTFORGE_DATA_DIR="$HF_DATA" hostforge deploy -sync-caddy <repo-v2>
   ```

3. **Quick error summary**:

   ```bash
   rg " ERR$| 5[0-9][0-9]$" /tmp/hf-phase8-cutover.log
   ```

Expected results:

- No sustained `ERR`/5xx window during cutover.
- Response changes from v1 to v2 after candidate passes health.

### 8.5 Failure-path reliability proof

Trigger one deliberately broken deploy (build failure or health failure):

```bash
HOSTFORGE_DATA_DIR="$HF_DATA" hostforge deploy -sync-caddy -container-port 9999 <repo-url>
curl -sS -o /dev/null -w "%{http_code}\n" "https://$HF_PUBLIC_APP_DOMAIN/"
```

Expected results:

- Candidate deployment marked `FAILED`.
- Prior successful deployment still serves `200`.
- No route flip to broken candidate.

### 8.6 Security baseline proof

Use unauthenticated requests against management endpoints (adjust URL paths if customized):

```bash
curl -sS -o /dev/null -w "%{http_code}\n" "$HF_SERVER_URL/api/projects"
curl -sS -o /dev/null -w "%{http_code}\n" "$HF_SERVER_URL/api/deployments"
```

Expected results: `401` or `403` without token.

Optional authenticated control:

```bash
curl -sS -H "Authorization: Bearer $HF_TOKEN" "$HF_SERVER_URL/api/projects"
```

Expected results: authenticated request succeeds.

Webhook negative test (invalid auth/signature) should be rejected by webhook endpoint with non-2xx.

### 8.7 Observability sanity proof

Checklist:

- Deployment steps are visible for both success and failure.
- Logs stream continuously during deploy.
- Failure reason is understandable without reading source code.

Capture:

- deployment IDs checked
- one success log excerpt
- one failure log excerpt
- UI/API location used for evidence

### 8.8 Evidence bundle and sign-off

Fill and store with release notes:

```text
Phase 8 summary:
- 8.1 VPS deploy: PASS/FAIL
- 8.2 Domain + HTTPS: PASS/FAIL
- 8.3 Golden matrix: PASS/FAIL
- 8.4 Zero-downtime: PASS/FAIL
- 8.5 Failure-path safety: PASS/FAIL
- 8.6 Security baseline: PASS/FAIL
- 8.7 Observability: PASS/FAIL

Blocking issues:
- ...

Sign-off:
- operator:
- reviewer:
- date:
```

### Suggested 72-hour execution rhythm

- **Day 1:** complete 8.1 and 8.2.
- **Day 2:** complete 8.3, 8.4, 8.5.
- **Day 3:** fix blockers, rerun failed checks, complete 8.6/8.7 and sign-off.
