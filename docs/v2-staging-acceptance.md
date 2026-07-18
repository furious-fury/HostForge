# HostForge v2 staging acceptance

Use this runbook on a staging VPS after installing a release that includes the Application -> Environment -> Service -> Deployment cutover. Do not mark the v2 integration complete from local tests alone.

## Operator context

```bash
export HF_SERVER_URL="https://hostforge.example.com"
export HF_TOKEN="<management-api-token>"
export HF_PUBLIC_APP_DOMAIN="app.example.com"
export VPS_HOST="root@server.example.com"
```

Record the HostForge commit/version, VPS image, Docker version, Caddy version, Railpack version, BuildKit worker output, operator, and test window.

The operator machine running the smoke script must have `curl` plus either `jq` or `python3`.

## 1. Upgrade and migration

1. Start from a populated pre-v2 database containing a project, deployment, container, domain, encrypted variables, GitHub installation references, and observability rows.
2. Run the fail-fast VPS update helper so installation stops if the pull or build fails.
3. Confirm `<database>.pre-application-model.bak` exists and is not tracked by Git.
4. Verify the server starts, the migration version advances, deployment IDs are unchanged, production owns the migrated branch/active release, and staging exists with no invented branch.
5. Confirm `/api/projects` and legacy PAT/SSH management endpoints return `404`.

```bash
ssh "$VPS_HOST" 'cd /opt/hostforge && HF_SERVER_URL="https://hostforge.example.com" ./scripts/vps-update-and-smoke.sh'
```

When upgrading a release that predates the helper, follow the one-time bootstrap command in [`docs/vps-update.md`](vps-update.md).

## 2. Read-only API and session smoke

Run the automated session/API smoke from a trusted operator machine:

```bash
HF_SERVER_URL="$HF_SERVER_URL" HF_TOKEN="$HF_TOKEN" ./scripts/v2-staging-api-smoke.sh
```

The script establishes an HTTP-only cookie session, reads settings/status/onboarding/GitHub/application/service/deployment/domain/variable/metrics/observability resources, logs out, and proves the protected API returns `401` afterward. Empty collection fields must be `[]`, never `null`.

## 3. Application and service hierarchy

In the UI:

1. Create an application and verify production and staging appear immediately.
2. Add two GitHub App-backed services from repositories visible to the selected installation.
3. Assign different production and staging branches.
4. Enable automatic deployment for one binding only.
5. Refresh nested URLs directly and use browser back/forward navigation.
6. Verify loading, empty, error/retry, success, validation, and pending-mutation states without fixture fallback.

Capture application, environment, and service IDs.

## 4. Deployment lifecycle and live logs

1. Trigger a manual production deployment and observe resumable live build logs.
2. Navigate away and return while the deployment continues.
3. Trigger a matching GitHub push and verify only matching service/environment bindings deploy.
4. Cancel one queued/building deployment.
5. Redeploy an exact successful commit.
6. Roll back to an earlier successful deployment and confirm a new deployment records `rollback_of`.
7. Trigger a failed health check while continuously probing the public URL; verify the prior successful release remains active.

```bash
while true; do
  code="$(curl -sS -o /dev/null -w "%{http_code}" "https://$HF_PUBLIC_APP_DOMAIN/" || echo ERR)"
  printf "%s %s\n" "$(date -Iseconds)" "$code"
  sleep 0.2
done | tee /tmp/hostforge-v2-cutover.log
```

No sustained `ERR` or `5xx` window is acceptable during successful cutover or failed-candidate retention.

## 5. Variables, domains, and runtime

1. Add application variables and a service override; verify only `value_last4` returns.
2. Replace and delete a secret through confirmation dialogs.
3. Import a valid `.env` file, then an invalid file; invalid input must not partially mutate state.
4. Add a domain targeting a service, run DNS checks, and verify Caddy validation happens before reload.
5. Exercise stop and restart confirmations and verify resulting desired/container state.
6. Delete a domain and verify routing removal or an explicit partial-failure warning.

## 6. Observability, settings, and safety

1. Verify host history, HTTP requests, deployment steps, platform events, and service metrics use real samples and freshness timestamps.
2. Verify stopped/unsupported/stale metric states are explicit.
3. Verify System Status is diagnostic-only and has no Docker/Caddy daemon restart control.
4. Exercise safe settings actions: public-IP detection, Caddy validation, route synchronization, and GitHub installation synchronization.
5. Create or reconnect a GitHub App and verify the callback exchanges its one-time code through `POST /api/github/app/manifest/exchange`.
6. Verify PAT, SSH-key, raw public container-port, account-management, and legacy project controls are absent.
7. Let a session expire or invalidate its cookie; verify redirect to `/login` preserves the intended nested route and successful login returns there.

## 7. Sign-off

Before sign-off, complete the managed-database matrix:

```bash
ssh "$VPS_HOST" 'cd /opt/hostforge && bash ./scripts/database-services-vps-audit.sh'
```

1. Provision PostgreSQL, MySQL, MariaDB, MongoDB, Redis, and Valkey in staging and verify no database port appears in `ss -lnt`.
2. From a bound staging application, write and read a known record; confirm an application in another environment cannot connect.
3. Restart each database and then restart Docker and `hostforge-server`; verify data, desired state, and ownership reconciliation.
4. Rotate credentials, confirm the previous password is rejected, redeploy the bound application, and confirm the new connection works.
5. Back up each engine to the configured R2/S3 destination and restore verified data as a new service.
6. Exercise replace-current with a deliberately invalid source restore and verify the safety backup rollback preserves the pre-restore data.
7. Delete one instance, restore it within seven days, then delete it again and confirm only the labelled volume is purged after the retention deadline.
8. Re-run `database-services-vps-audit.sh` after provisioning and after the Docker restart; attach its disk, limit, network, and exposure output to the acceptance record.
9. When a same-version catalog digest update is available, verify patch preflight requires a successful backup no older than 24 hours, then confirm volume/alias reuse and previous-digest rollback on a failed health check.
10. Complete the PostgreSQL public gateway matrix in [HostForge_Database_Gateway_VPS_Acceptance.md](HostForge_Database_Gateway_VPS_Acceptance.md) while the feature remains staging-only.

```text
Migration and backup: PASS/FAIL
Session/API smoke: PASS/FAIL
Application/service hierarchy: PASS/FAIL
Manual and webhook deploys: PASS/FAIL
Live logs/reconnect/cancel: PASS/FAIL
Failed-health retention: PASS/FAIL
Redeploy and rollback audit: PASS/FAIL
Variables/domains/Caddy HTTPS: PASS/FAIL
Runtime stop/restart: PASS/FAIL
Observability/status/settings: PASS/FAIL
GitHub App manifest exchange: PASS/FAIL
Nested routes/session expiry: PASS/FAIL
No fixture or inert control observed: PASS/FAIL

Blocking issues:
- ...

Operator:
Reviewer:
Date:
```
