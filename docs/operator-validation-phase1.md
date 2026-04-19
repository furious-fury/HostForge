# Operator validation — Detailed backlog §1

This runbook implements **[`task_list.md`](../task_list.md) → Detailed backlog → 1. Operator validation and exit criteria**: **1.1** (Docker), **1.2** (HTTPS + restarts), **1.3** (zero-downtime cutover).

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
