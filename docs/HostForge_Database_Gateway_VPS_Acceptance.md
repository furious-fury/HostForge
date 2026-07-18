# HostForge PostgreSQL gateway staging acceptance

Use this runbook on a staging VPS release that contains the PostgreSQL v1 gateway. Do not enable the gateway by default or declare the feature complete from local tests alone. Record the HostForge commit, VPS image, Docker/Caddy/PgBouncer versions, operator, source IPv4/IPv6 addresses, and test window.

The connection URLs returned by HostForge are secrets. Keep them in environment variables, never paste them into tickets or shell history, and destroy the test connections after acceptance.

## 1. Prerequisites and activation

### Phase A: deploy disabled

1. Confirm `/etc/hostforge/hostforge.env` explicitly contains `HOSTFORGE_DATABASE_GATEWAYS_ENABLED=false`. Do not rely only on the compiled default for this first deployment.
2. Configure `HOSTFORGE_POSTGRES_GATEWAY_IMAGE` with a reviewed digest-pinned image containing both PgBouncer and `psql`, and set `HOSTFORGE_POSTGRES_GATEWAY_VERSION` to its version, which must be at least 1.25.2.
3. Verify TCP/5432 is free. The privileged installer fails closed for any foreign listener.
4. Deploy the candidate release with the standard update helper and run the isolation audit.
5. Verify HostForge is healthy, gateway APIs report the feature disabled, no gateway container exists, and no process listens on TCP/5432.

```bash
ssh "$VPS_HOST" 'grep -q "^HOSTFORGE_DATABASE_GATEWAYS_ENABLED=false$" /etc/hostforge/hostforge.env'
ssh "$VPS_HOST" 'cd /opt/hostforge && ./scripts/vps-update-and-smoke.sh'
ssh "$VPS_HOST" 'cd /opt/hostforge && HF_EXPECT_DATABASE_GATEWAY_STATE=absent bash ./scripts/database-services-vps-audit.sh'
ssh "$VPS_HOST" 'if ss -lntH "sport = :5432" | grep -q .; then echo "unexpected TCP/5432 listener" >&2; exit 1; fi'
```

A listener, gateway container, or successful gateway mutation during Phase A is a blocking failure. Private database containers must still have no host port bindings.

### Phase B: enable on staging only

1. Point `postgres.<platform-domain>` A and, when supported, AAAA records at the staging VPS.
2. Confirm Caddy issued a certificate whose SAN exactly matches the reserved hostname and TCP/5432 is allowed through both host and provider firewalls.
3. Change only the staging VPS environment to `HOSTFORGE_DATABASE_GATEWAYS_ENABLED=true`, leave operation concurrency at one, and restart `hostforge-server`.
4. Confirm the gateway status API reports the feature enabled but TCP/5432 remains unused before lazy provisioning.
5. Provision one healthy PostgreSQL database instance for the test environment. Keep Production and Staging instance IDs in the acceptance record.
6. In Database settings, provision the PostgreSQL gateway. Wait for the durable operation and TLS/auth probe to report success.
7. Run the isolation audit again. It must show exactly one hardened gateway container and only its owned port 5432.

```bash
ssh -t "$VPS_HOST" 'sudoedit /etc/hostforge/hostforge.env'
ssh "$VPS_HOST" 'systemctl restart hostforge-server && systemctl is-active --quiet hostforge-server'
ssh "$VPS_HOST" 'if ss -lntH "sport = :5432" | grep -q .; then echo "gateway activated before provisioning" >&2; exit 1; fi'
# Provision the gateway in Database settings and wait for its operation to succeed.
ssh "$VPS_HOST" 'cd /opt/hostforge && HF_EXPECT_DATABASE_GATEWAY_STATE=active bash ./scripts/database-services-vps-audit.sh'
```

Do not make `true` the repository, installer, or production default after Phase B. The flag remains a staging-only override until every acceptance section passes and the test connections are revoked.

## 2. Permission and TLS fixture

Run this phase from an allowed operator source. Create only a migration connection first, restricted to the operator's exact `/32` or `/128` CIDR, and reveal its current URL.

```bash
export HF_GATEWAY_MIGRATION_URL='<secret migration URL>'
export HF_GATEWAY_TEST_SCHEMA="hf_gateway_accept_$(date -u +%Y%m%d%H%M%S)"
./scripts/database-gateway-vps-acceptance.sh prepare
```

The prepare phase connects with `sslmode=verify-full`, proves the generated role has no administrative attributes, discovers its single application-owner membership, and creates a seed table as that owner.

After prepare succeeds, create read-only and read-write connections for the same database and allowed source. Creating them after the seed table is required: it lets the verify phase distinguish current-object reconciliation from future-object default privileges.

```bash
export HF_GATEWAY_READ_ONLY_URL='<secret read-only URL>'
export HF_GATEWAY_READ_WRITE_URL='<secret read-write URL>'
./scripts/database-gateway-vps-acceptance.sh verify
unset HF_GATEWAY_MIGRATION_URL HF_GATEWAY_READ_ONLY_URL HF_GATEWAY_READ_WRITE_URL
```

The verify phase checks TLS, generated-role separation and safety, current and future schema/table/sequence grants, allowed DML, denied DML, denied DDL, owner-equivalent migration DDL, and fixture cleanup. Set `HF_GATEWAY_KEEP_FIXTURES=true` only when the schema is needed for lifecycle inspection.

## 3. Client compatibility

Use a fresh read-only URL from an allowed source. The fixture pins the versions in its package and requirements files; `npm ci` uses the committed lockfile.

```bash
export DATABASE_URL='<secret read-only URL>'
cd scripts/database-gateway-client-smoke
npm ci
npm run smoke

python3 -m venv .venv
. .venv/bin/activate
python -m pip install --requirement requirements.txt
python smoke-sqlalchemy.py
deactivate
unset DATABASE_URL
```

This proves direct `pg`, Drizzle, Prisma 6.19.3 with its matching PostgreSQL adapter, and SQLAlchemy/psycopg connections. The profile runner already proves `psql`. Every smoke checks that the backend reports an active TLS session; the URL guard requires `sslmode=verify-full` so hostname and certificate verification cannot silently degrade.

Run the user's deployed Next.js CRUD test application against a read-write URL as the application-level check. Create, read, update, and delete a record, restart the application, and repeat the read.

## 4. IPv4, IPv6, and CIDR denial

Use two machines or egress paths with recorded source addresses.

1. Create `/32` and `/128` connections for the allowed IPv4 and IPv6 sources and prove each connects.
2. From the denied source, attempt the same URLs with a ten-second timeout. Both must fail before PostgreSQL authentication succeeds.
3. Change a connection CIDR from the allowed source to the denied source. Poll the operation to success and prove the old active session is terminated and cannot reconnect.
4. Verify `0.0.0.0/0` and `::/0` are rejected without the boolean API guard and the typed UI phrase `ALLOW PUBLIC ACCESS`.

```bash
PGCONNECT_TIMEOUT=10 PGDATABASE="$HF_GATEWAY_DENIED_URL" \
  psql -X --no-psqlrc --set=ON_ERROR_STOP=1 --command 'SELECT 1'
```

A successful command from the denied source is a blocking failure. Do not print the URL in the acceptance record.

## 5. Rotation and targeted revocation

Keep a long-running session open for connection A and another for unrelated connection B.

1. Rotate A with a one-hour grace period and reveal N+1.
2. Prove N and N+1 both connect during grace, while B remains connected.
3. Rotate another connection with zero grace and prove its N credential immediately fails while N+1 succeeds.
4. Disable A. Its sessions must end and both generations must reject new logins; B must remain connected.
5. Re-enable A and prove only its current generation returns.
6. Revoke A with typed confirmation. Prove its sessions end, its URLs fail, its ciphertext is no longer revealable, and B remains connected.
7. For the one-hour rotation, wait for retirement or temporarily use a shorter accepted grace value in a separate connection. Prove only N stops working after the deadline.

Record connection IDs and generations, never usernames, passwords, or URLs.

## 6. Lifecycle and reconciliation

For a connection that remains active:

1. Stop the database. The route must disable and active sessions must terminate.
2. Start it. Access must resume only after database health and the external probe pass.
3. Restart `hostforge-server`; the already healthy PgBouncer container must continue serving connections.
4. Restart Docker, then HostForge. Container and link-network ownership must reconcile without exposing a database port.
5. Perform a supported same-version database upgrade. The alias must pause, reconnect to the replacement container, pass health, and resume.
6. Renew or replace the Caddy certificate and wait for the poller. The endpoint fingerprint/config generation must advance without dropping unrelated sessions.
7. Delete the database. All external credentials and its link route must revoke before retained deletion.
8. Restore retained data. Public access must stay off until a new external connection is explicitly created.
9. Attempt a platform-domain change while any external connection is active; it must fail with the guarded error. Revoke all connections and tear down the gateway before testing the allowed change path.

Run the audit after each restart/upgrade/delete boundary:

```bash
ssh "$VPS_HOST" 'cd /opt/hostforge && bash ./scripts/database-services-vps-audit.sh'
```

## 7. Sign-off

```text
HostForge commit/version:
VPS/Docker/Caddy/PgBouncer versions:
DNS A/AAAA and certificate SAN: PASS/FAIL
Gateway hardening and only-owned-5432 audit: PASS/FAIL
psql TLS/profile/current/future object runner: PASS/FAIL
pg client: PASS/FAIL
Drizzle client: PASS/FAIL
Prisma client: PASS/FAIL
SQLAlchemy client: PASS/FAIL
Next.js CRUD application: PASS/FAIL
Allowed IPv4 and IPv6: PASS/FAIL
Denied IPv4 and IPv6: PASS/FAIL
CIDR update terminates old source: PASS/FAIL
Rotation grace and zero-grace: PASS/FAIL
Disable/enable/targeted revoke: PASS/FAIL
Database stop/start/upgrade: PASS/FAIL
HostForge and Docker restart: PASS/FAIL
Certificate renewal/reload: PASS/FAIL
Retained delete/restore stays private: PASS/FAIL
Platform-domain guard and teardown: PASS/FAIL

Blocking issues:
- ...

Operator:
Reviewer:
Date:
```

Only after every row passes may a separate reviewed change switch the default gateway feature flag to true.
