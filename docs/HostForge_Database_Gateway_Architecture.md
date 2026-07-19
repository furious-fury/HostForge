# HostForge Database Gateway Architecture

Status: implementation contract
Version: PostgreSQL v1 on a multi-engine control-plane foundation
Default feature state: disabled (`HOSTFORGE_DATABASE_GATEWAYS_ENABLED=false`)

## 1. Scope and locked decisions

HostForge exposes explicitly selected database instances to external clients without publishing any database container port. The control plane is engine-neutral; PostgreSQL is the only public data-plane adapter in v1.

The following decisions are part of the v1 contract:

- Public access is configured per `database_instance`, preserving Production/Staging isolation.
- One HostForge-managed PostgreSQL gateway is provisioned lazily at `postgres.<platform-domain>:5432`.
- SQLite is the source of truth. PgBouncer containers, networks, roles, and generated files are derived state.
- The gateway feature flag is off by default. When enabled, PostgreSQL database creation automatically queues one isolated read/write external connection per selected environment, restricted to the creator's exact public IP; all access still requires TLS, SCRAM credentials, at least one CIDR, a fixed permission profile, and connection limits.
- Credential secrets are encrypted at rest and may be deliberately re-displayed through a no-store endpoint.
- Rotation creates a new credential generation before retiring the old one. The default grace period is 24 hours and the allowed range is 0–168 hours.
- PgBouncer uses session pooling. HA, transaction pooling, custom public hostnames, arbitrary grants, client certificates, and non-PostgreSQL public adapters are outside v1.

## 2. System boundaries

```text
External PostgreSQL client
        | TLS 1.2+, SCRAM, source CIDR
        v
postgres.<platform-domain>:5432
        |
        v
HostForge PgBouncer gateway
        | one isolated link network per enabled database instance
        +---- hf_<instance-id> ----> private PostgreSQL container

HostForge control plane
        | desired state, grants, encrypted credentials, operations
        v
SQLite
```

The gateway is not an HTTP service and Caddy does not proxy PostgreSQL traffic. Caddy remains the ACME certificate issuer; HostForge synchronizes only the validated certificate/key pair for the reserved PostgreSQL hostname into the gateway TLS directory.

Private application bindings continue to use environment Docker networks and never traverse PgBouncer. Enabling external access cannot add a published port to a database container.

## 3. Threat model and security invariants

The design assumes a single trusted HostForge operator on one VPS and treats the public network, external clients, user-provided names, and database traffic before authentication as hostile. Docker daemon or host-root compromise is outside the isolation boundary; gateway compromise must still not provide the Docker socket or unrelated application/database networks.

Security invariants:

1. Only the owned gateway container may publish host TCP/5432.
2. Database containers publish no host ports.
3. A gateway link network contains exactly the gateway and one target PostgreSQL container.
4. Public access is deny-by-default. Rendered HBA files end with IPv4 and IPv6 reject rules.
5. A route requires a matching enabled connection, active credential generation, allowed source CIDR, TLS, and SCRAM authentication.
6. Generated aliases and PostgreSQL role names are immutable lowercase identifiers. User-facing names never become protocol identifiers.
7. Plaintext passwords, SCRAM verifiers, TLS keys, and complete connection URLs never appear in normal API details, events, logs, metrics, operation errors, or process arguments.
8. Secrets are encrypted using the existing HostForge sealing key. Generated files are mode `0600`, written atomically, and mounted read-only into the gateway.
9. Revocation is scoped to the selected external connection and its credential generations; unrelated routes and roles remain connected.
10. Gateway creation fails closed on missing DNS, unavailable/invalid TLS, an occupied port, an unpinned or insecure image, or a failed probe.

PgBouncer behavior must follow its official [authentication, HBA, and TLS configuration contract](https://www.pgbouncer.org/config.html), [session-pooling compatibility matrix](https://www.pgbouncer.org/features.html), and [reload and targeted client-control commands](https://www.pgbouncer.org/usage.html). Images below PgBouncer 1.25.2 are rejected because 1.25.2 includes a security fix affecting `KILL_CLIENT`; see the [PgBouncer 1.25.2 release notice](https://www.pgbouncer.org/2026/05/pgbouncer-1-25-2).

## 4. Persistence model

Migration `0022_database_gateway_foundation.sql` adds the following tables.

### `database_gateway_endpoints`

One row per engine:

- `engine` primary key
- public `hostname` and `port`
- immutable digest-pinned `image_ref`, container name/ID, and ingress network name
- `desired_status`: `absent`, `active`, or `deleting`
- `observed_status`: `absent`, `provisioning`, `active`, `degraded`, `failed`, or `deleting`
- TLS state: certificate fingerprint, expiry, and last synchronization time
- desired, rendered, and applied configuration generations
- sanitized last error code/message and timestamps

### `database_gateway_routes`

At most one route per `database_instance`:

- immutable alias `hf_<normalized-instance-id>`
- globally unique backend alias and link-network name
- desired/observed route state
- route/credential backend budgets
- timestamps and sanitized last error

Routes cascade with instance deletion. External access is never restored by restoring retained database data: the deletion workflow revokes connections and removes the route before the database enters retained deletion.

### `database_external_connections`

A user-visible, logical access grant belonging to one route:

- name, permission profile, optional RFC3339 expiry
- status: `pending`, `active`, `disabled`, `expired`, `rotating`, `revoking`, `revoked`, or `failed`
- per-credential client limit
- approximate usage metadata
- timestamps and sanitized last error

The permission profile is one of `read_only`, `read_write`, or `migration`.

### `database_external_credentials`

Versioned credential generations belonging to a connection:

- immutable generated role `hfc_<normalized-credential-id>`
- encrypted password and exact PostgreSQL SCRAM verifier
- positive generation number
- state: `active`, `grace`, `revoked`
- grace deadline, usage metadata, and timestamps

Only active or non-expired grace credentials may be rendered into PgBouncer. Revocation clears ciphertext while preserving non-secret audit metadata.

### `database_external_connection_cidrs`

Canonical network prefixes parsed with Go's `net/netip` package. The schema enforces uniqueness per connection. Each connection requires at least one CIDR at activation time; this cross-row rule is enforced transactionally by the repository.

### `database_gateway_operations`

A durable, leased, retryable operation queue following the existing database-operation worker contract:

- optional engine, route, connection, and credential scope
- operation type: `provision_gateway`, `teardown_gateway`, `create_connection`, `update_connection`, `disable_connection`, `enable_connection`, `rotate_connection`, `revoke_connection`, `expire_connection`, or `reconcile_route`
- queued/running/success/failed/cancelled status and progress
- lease owner/expiry, attempt count, requested grace period, actor, sanitized errors, and timestamps

Mutations persist desired state and an operation in one transaction. Workers lease jobs, renew or complete leases, and requeue expired leases after process restarts.

## 5. Identifier rules

Identifiers are generated from UUID/ULID-style HostForge IDs by lowercasing and removing every character outside `[a-z0-9]`:

- route alias: `hf_<instance-id>`
- credential role: `hfc_<credential-id>`
- backend alias: `hfb_<instance-id>`

Names must fit PostgreSQL's 63-byte identifier limit. The normalized ID must be non-empty; collisions are prevented by unique constraints. Aliases and role names never change when a user renames a connection, service, application, or environment.

## 6. Control-plane interfaces

The orchestration layer depends on an engine adapter rather than PgBouncer-specific repository code. An adapter provides:

```go
type DatabaseGatewayAdapter interface {
    Engine() string
    Endpoint(context.Context, GatewayEndpointRequest) (GatewayEndpointSpec, error)
    ConnectionURL(ConnectionURLRequest) (string, error)
    ProvisionRole(context.Context, RoleRequest) (RoleMaterial, error)
    ReconcilePermissions(context.Context, PermissionRequest) error
    RevokeRole(context.Context, RevokeRoleRequest) error
    Render(context.Context, GatewayRenderRequest) (GatewayGeneration, error)
    Validate(context.Context, GatewayGeneration) error
    Reload(context.Context, GatewayGeneration) error
    Probe(context.Context, GatewayProbeRequest) error
    Terminate(context.Context, TerminationRequest) error
}
```

PostgreSQL is the sole registered v1 adapter. Requests for MySQL, MariaDB, MongoDB, Redis, or Valkey return `external_access_engine_unsupported`. The generic repository and APIs still expose their adapter availability so later protocol gateways use the same desired-state and operation machinery.

## 7. Generated state and activation

Gateway generations live below `<data-dir>/database-gateways/postgresql/generations/<generation>/`. Each contains the minimum runtime files:

- `pgbouncer.ini`
- `userlist.txt` containing usernames and exact SCRAM verifiers only
- `pg_hba.conf`
- `databases.ini` or an equivalent generated database section
- synchronized `server.crt` and `server.key`
- a non-secret manifest containing checksums and generation metadata

Files are written into a new private directory, fsynced where supported, chmod `0600`, and validated before activation. `current` is replaced atomically only after validation. SQLite generations remain authoritative; every file can be rebuilt from SQLite and decrypted secrets.

Applying a generation follows: render, validate, stage, atomically activate, issue PgBouncer `RELOAD`, probe, then record the applied generation. On failure HostForge reactivates the previous valid generation and leaves existing credentials usable.

## 8. PostgreSQL/PgBouncer data plane

### Container and networks

The gateway runs as a separately labelled HostForge resource using an operator-configured digest-pinned PgBouncer image whose declared version is at least 1.25.2. The image must contain both the `pgbouncer` and `psql` executables because HostForge uses `psql` over the Unix administration socket for reload, probes, and targeted session management. The catalog must not expose an image below that floor.

The container has:

- `restart-unless-stopped`
- read-only root filesystem
- all Linux capabilities dropped
- `no-new-privileges`
- no Docker socket
- tmpfs runtime directories
- only generated config and TLS mounts
- the owned ingress bridge and active per-instance link networks
- public TCP/5432; no other published ports

The ingress network contains only PgBouncer. Every active route gets a HostForge-owned link network containing only PgBouncer and the target PostgreSQL container. The target is addressed by the globally unique `hfb_<instance-id>` backend alias. Reconciliation reconnects a replacement container after database restart/upgrade and deletes empty link networks after route removal.

### PgBouncer configuration

The adapter renders:

```ini
[pgbouncer]
pool_mode = session
auth_type = hba
auth_file = /etc/pgbouncer/userlist.txt
auth_hba_file = /etc/pgbouncer/pg_hba.conf
client_tls_sslmode = require
client_tls_protocols = secure
max_client_conn = 200
admin_users = hostforge_gateway_admin
unix_socket_dir = /run/pgbouncer
```

Administration is reachable only through the mounted Unix socket. The local `trust` identity is present in `userlist.txt` with a fixed, non-login SCRAM-formatted existence sentinel because PgBouncer requires trusted users to exist in `auth_file`; no password corresponds to that sentinel. Public HBA rules explicitly reject the `pgbouncer` administrative database and the admin role. For every active route/credential/CIDR tuple the renderer emits one `hostssl` SCRAM rule. Final IPv4 and IPv6 reject rules deny all unmatched clients.

### Credentials and SCRAM

The PostgreSQL adapter generates a 32-byte cryptographically random base64url password. It creates a role with explicit safe attributes:

```sql
LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS
```

After setting the password, HostForge queries PostgreSQL for its exact SCRAM verifier. The password and verifier are separately encrypted at rest; only the verifier is written to `userlist.txt`. HostForge never derives a substitute verifier because PgBouncer SCRAM pass-through requires the exact backend secret.

### Permission profiles

`read_only` grants database connect, usage on non-system schemas, reads on current tables/views/sequences as appropriate, and owner default privileges for future objects.

`read_write` includes read-only rights plus insert/update/delete and sequence usage/select. It does not grant truncate, schema creation, DDL, ownership, or administrative attributes.

`migration` grants membership in a HostForge-managed application-owner role for the database and activates that owner as the credential's database-specific login role. This ensures migration-created objects are owner-owned and receive the owner's future-object default privileges. Profile changes and revocation reset the login role before membership removal. The credential is prominently described as owner-equivalent but still never receives superuser, CREATEDB, CREATEROLE, REPLICATION, or BYPASSRLS.

Profile reconciliation revokes obsolete current grants, default privileges, and membership before applying the selected profile. Disabling uses `NOLOGIN` but retains grants. Revocation removes current grants, owner default privileges, and membership before dropping the credential role.

### Pool budgets

| Resource preset | Route backend connections | Credential backend connections |
| --- | ---: | ---: |
| development | 10 | 5 |
| standard | 25 | 10 |
| performance | 50 | 20 |
| custom | `clamp(10 × memory GiB, 10, 50)` | `min(10, route)` |

The global client capacity is 200 and each credential has a client capacity of 20. Rendering must be deterministic and reject totals or values that violate these limits.

## 9. TLS, DNS, and firewall lifecycle

The reserved hostname is `postgres.<platform-domain>`. Provisioning verifies that its public A/AAAA answers match configured/detected VPS addresses. A missing platform domain or mismatch fails closed with actionable DNS guidance.

HostForge adds the reserved hostname to managed Caddy certificate configuration without making Caddy a PostgreSQL proxy. It locates only the matching SAN certificate/key pair, validates the key pair and hostname, copies it atomically into the generation TLS directory, and records fingerprint/expiry. Renewal polling synchronizes a changed pair and reloads PgBouncer. Rate-limited warnings monitor an applied certificate approaching expiry, and expired or otherwise invalid material blocks activation.

Privileged install/update scripts reserve inbound UFW TCP/5432 while no listener exists until gateway activation. Installations without UFW receive manual firewall guidance. Gateway setup checks the host port immediately before container creation and fails with `database_gateway_port_occupied` if another listener owns it.

Platform-domain changes are rejected while any gateway has active external connections. The operator must revoke connections and tear down gateways first.

## 10. State machines and lifecycle

### Endpoint

```text
absent -> provisioning -> active -> deleting -> absent
              |             |
              v             v
            failed <---- degraded
```

### Connection

```text
pending -> active <-> disabled
             |  \       |
             |   -> rotating -> active
             |             |
             -> expired    |
             \-------------+-> revoking -> revoked
```

`revoked` is terminal. A failed operation preserves the safest previous usable state; rotation failure never invalidates generation N.

Lifecycle contracts:

- **Create:** persist route/connection/operation; provision role and grants; fetch and encrypt the verifier; attach networks; render/validate/activate; reload; probe TLS/auth; mark active.
- **Disable/expire:** remove HBA access; reload; terminate only matching PgBouncer clients and PostgreSQL sessions; set roles `NOLOGIN`; mark disabled/expired.
- **Enable:** reconcile role attributes, grants, network, and generation; probe before marking active.
- **Rotate:** create N+1 first and probe it; move N to grace for 0–168 hours (default 24); after the deadline remove N from HBA, terminate only N sessions, revoke grants, drop the old role, and clear ciphertext. A failure leaves N active.
- **Revoke:** terminally deny new traffic, reload, terminate only roles owned by the connection, revoke grants/default privileges, drop roles, clear ciphertext, and retain non-secret audit metadata.
- **Stop database:** disable its route and terminate route sessions before stopping. Start re-enables only after database health succeeds.
- **Restart/upgrade:** pause the alias, replace/reconnect the database container, verify health, and resume.
- **Delete database:** revoke every external connection and remove the route/link before retained deletion. Restoring retained storage does not restore public access.
- **Control-plane downtime:** an already healthy `restart-unless-stopped` PgBouncer container continues serving its last applied generation.

An expiry claimant periodically queues `expire_connection` for active connections whose expiry has passed. A grace claimant queues targeted retirement for credentials whose grace deadline has passed. Both claims must be idempotent.

## 11. API contract

All endpoints require existing HostForge management authentication.

- `GET /api/database-gateways/{engine}` returns feature/adapter availability, endpoint/TLS/config state, DNS guidance, and sanitized errors.
- `POST /api/applications/{id}/database-services` automatically queues initial PostgreSQL external connections when the gateway feature is enabled and returns their durable operations without secret material.
- `POST /api/database-gateways/{engine}` lazily provisions the gateway and returns `202` with an operation.
- `DELETE /api/database-gateways/{engine}` requires typed confirmation, rejects active connections, and returns `202`.
- `GET /api/database-gateway-operations/{id}` returns durable progress.
- `GET /api/database-instances/{id}/external-access` returns its environment-scoped route, gateway summary, connections, and non-secret credential generations.
- `POST /api/database-instances/{id}/external-connections` accepts `name`, `profile`, `cidrs`, optional RFC3339 `expires_at`, and `confirm_open_access`; returns `202`.
- `PATCH /api/database-external-connections/{id}` updates name/profile/CIDRs/expiry and returns `202`.
- `POST /api/database-external-connections/{id}/disable`, `/enable`, `/rotate`, or `/revoke` returns `202`; rotate accepts `grace_period_hours`.
- `POST /api/database-external-connections/{id}/credentials` returns the current username, password, alias, endpoint, and percent-encoded URL with `200` and `Cache-Control: no-store`.

CIDRs are parsed and masked using `net/netip`, duplicate canonical prefixes are removed, and at least one is required. `0.0.0.0/0` or `::/0` requires `confirm_open_access=true`; the UI additionally requires the exact phrase `ALLOW PUBLIC ACCESS`.

Stable errors include:

- `database_gateways_disabled`
- `external_access_engine_unsupported`
- `database_gateway_platform_domain_required`
- `database_gateway_dns_mismatch`
- `database_gateway_port_occupied`
- `database_gateway_tls_unavailable`
- `invalid_external_access_cidr`
- `invalid_external_access_profile`
- `invalid_external_access_expiry`
- `external_access_open_confirmation_required`
- `invalid_external_connection_state`
- `database_gateway_has_active_connections`

Secrets are omitted from every normal response. The reveal response is never cached and should not be retained in frontend query caches.

## 12. Settings UI contract

Database Settings contains a distinct **Internal application connections** section and a **Public external access** section.

For PostgreSQL, the public section shows:

- an automatic gateway status card with the reserved hostname, A/AAAA guidance, TLS fingerprint/expiry, container health, and desired/rendered/applied generation; there is no separate setup action
- environment-grouped instance cards so Production and Staging cannot be confused
- a create dialog with connection name, permission profile, canonical CIDR chips, current-IP shortcut, optional expiry, and warnings
- connection cards with status, profile, CIDRs, current generation, grace countdown, approximate last use, and reveal/copy/rotate/disable/enable/revoke actions
- polling while gateway operations are queued/running and final success/error toasts
- a database-creation progress dialog that waits for the private instance and gateway route, then automatically reveals every environment's initial URL, username, password, and alias with no-store handling

Migration access and open CIDRs display prominent warnings. Open-network creation requires typing `ALLOW PUBLIC ACCESS`. Connection revocation and global gateway teardown require typed confirmation.

Other engines show the multi-engine foundation and `External adapter not available yet`; no unusable creation form is rendered.

## 13. Observability and redaction

Metrics may report endpoint status, route/connection counts, generation numbers, operation duration/retries, certificate days remaining, pool utilization, and approximate last use. Labels must use non-secret IDs, never usernames or URLs where avoidable.

Audit/platform events record actor, operation, engine, route/connection ID, previous/new status, profile, canonical CIDRs, and timestamps. They must not contain passwords, verifiers, key material, URLs, SQL containing secrets, or PgBouncer auth lines.

Errors crossing repository/service/API boundaries are mapped to stable codes and sanitized messages. Adapter command output is redacted before persistence.

## 14. Rollout

1. Ship schema, APIs, adapter, UI, installer changes, and tests with `HOSTFORGE_DATABASE_GATEWAYS_ENABLED=false`.
2. Configure a digest-pinned PgBouncer >=1.25.2 image and enable the flag on a staging VPS.
3. Verify DNS/TLS/firewall setup and run repository, adapter, Docker/Caddy, API/UI, update, smoke, and VPS acceptance suites.
4. Audit that only the owned gateway publishes 5432 and database containers publish nothing.
5. Change the default to true only in a later reviewed change after the full matrix passes. This document does not authorize silently changing the initial default.

## 15. Test and acceptance contract

Repository tests cover constraints/cascades, alias/role uniqueness, canonical CIDRs, encrypted fields, operation leases/requeue, generations, expiry and grace claims, and transactional desired-state changes.

Adapter tests cover deterministic/escaped config and HBA output, exact SCRAM handling, all permission profiles including future-object defaults, percent-encoded URLs, redaction, atomic generations, pool limits, rotation compensation, and targeted revocation.

Docker/Caddy tests cover owned network labels/membership, gateway-only port publication, hardening/no socket, restart reconciliation, link cleanup, DNS/SAN/key validation, renewal reload, occupied-port rejection, and Caddy rollback.

API/UI tests cover every endpoint/state/error, no-store reveal, guarded open CIDRs, environment isolation, operation polling, setup/empty/error states, grace UX, and inaccessible unsupported-engine actions.

The staging VPS acceptance matrix requires:

The executable profile/TLS runner, current client fixtures, lifecycle procedure, and evidence template are maintained in [HostForge_Database_Gateway_VPS_Acceptance.md](HostForge_Database_Gateway_VPS_Acceptance.md).

- `psql`, Prisma, Drizzle, `pg`, and SQLAlchemy connect with `sslmode=verify-full`.
- Allowed IPv4/IPv6 sources connect; denied sources fail.
- Every profile permits and rejects the intended SQL for current and future objects.
- N and N+1 work during rotation grace; only N is revoked after grace.
- Disable/revoke kills only the targeted connection sessions.
- HostForge, Docker, PgBouncer, database restart/upgrade, certificate renewal, retained deletion, and restore reconcile safely.
- Audits prove database containers expose no host ports and only the owned gateway listens on 5432.

## 16. Non-goals

V1 does not provide high availability, PgBouncer transaction pooling, custom gateway hostnames or ports, arbitrary user-authored SQL grants, mutual TLS/client certificates, private overlay networking, per-tenant VPS isolation, or public adapters for MySQL/MariaDB/MongoDB/Redis/Valkey.

These are deliberate constraints, not incomplete UI options. Later adapters must preserve the same source-of-truth, operation, lifecycle, redaction, and deny-by-default contracts.
