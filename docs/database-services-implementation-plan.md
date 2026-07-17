# Database services implementation plan

## Purpose

This document defines how HostForge will add persistent database services without treating them as repository builds or ordinary application deployments.

Database services belong to an application and have one isolated instance per environment. A Production database and a Staging database must never share a container, volume, credentials, backup history, or network identity.

## Confirmed product decisions

- The initial supported catalog is PostgreSQL, MySQL, MariaDB, MongoDB, Redis, and Valkey.
- Every application environment receives a separate database instance.
- Database instances are reachable only by HostForge services in the same environment.
- HostForge can inject a generated connection URL into selected application services.
- Backup destinations support a streamlined Cloudflare R2 connection and generic S3-compatible storage.
- Deleting a database stops it immediately and retains its data volume for seven days before permanent purge.
- CPU and memory are selected through Development, Standard, Performance, or Custom resource presets.
- Public database access is explicitly deferred until HostForge can provide authenticated TCP ingress, database-aware TLS, firewall controls, auditability, and copyable external credentials.

## Scope

### Included in the first complete release

- Database setup wizard and engine/version selection.
- Persistent Docker volumes.
- Private environment-scoped container networking.
- Generated database name, username, password, and connection URL.
- Automatic connection-variable injection into selected application services.
- Start, stop, restart, credential rotation, and safe deletion.
- Health checks, status, runtime logs, CPU, memory, network, and storage-usage reporting.
- Manual and scheduled logical backups.
- Restore to the current instance or to a new database service.
- Cloudflare R2 and generic S3-compatible backup destinations.
- Seven-day volume retention and an operator-visible purge queue.
- Platform events and operation history for provisioning, backups, restores, rotations, and deletion.

### Explicitly excluded

- Public TCP endpoints.
- High availability, clustering, replicas, automatic failover, and multi-node databases.
- Cross-VPS private networking.
- Point-in-time recovery and continuous WAL/binlog archiving.
- Automatic major-version upgrades.
- Importing an externally hosted database as a managed HostForge database.
- Oracle Database and Microsoft SQL Server.
- Hard portable volume quotas. Docker CPU and memory limits are enforceable, but reliable volume quotas require filesystem or storage-driver support.

## Architectural rules

1. A database is a persistent resource, not a Railpack deployment.
2. Application deployments may be replaced freely; a database volume may not.
3. Container removal and volume removal must be separate operations.
4. Database containers must not publish a host port in the first release.
5. Only containers attached to the same HostForge environment network may resolve and reach the database.
6. Credentials and backup-provider secrets must be encrypted at rest and must never appear in logs or normal API responses.
7. Every destructive operation must be resumable or safely retryable.
8. Engine images and supported versions must come from a server-side catalog, not arbitrary user-provided image names.

## Data model

Add a migration after the current migration set with the following concepts.

### Extend `services`

Add `service_type` with supported values:

- `application`
- `database`

Existing records migrate to `application`.

Repository, build, branch, domain, and deployment validation must run only for application services. Database-specific configuration belongs in separate tables rather than adding nullable engine fields throughout the application-service model.

`service_environments` remains the release binding for application services. Database services use `database_instances` for their environment-specific state. Application summaries, service lists, health counts, and API DTOs must become service-type aware instead of assuming every service has a Git branch and deployment.

### `database_services`

One row per database service:

- `service_id`
- `engine`
- `default_version`
- timestamps

The service remains the common application-level identity used by navigation, events, and permissions.

### `database_instances`

One row per database service and application environment:

- `id`
- `service_id`
- `environment_id`
- `engine_version`
- immutable, digest-pinned `image_ref`
- `container_id`
- stable private `network_alias`
- engine `internal_port`
- `volume_name`
- `resource_preset`
- `cpu_limit_millis`
- `memory_limit_bytes`
- `desired_state`
- observed `status`
- health message and last health-check time
- last observed storage usage
- `deleted_at`
- `purge_after`
- timestamps

Enforce uniqueness for `(service_id, environment_id)`, network aliases within an environment, container names, and volume names.

### `database_credentials`

One row per instance:

- `database_instance_id`
- generated database name
- generated username
- encrypted password
- encrypted engine-administrator password used only for bootstrap, maintenance, restore, and application-password rotation
- credential generation number
- rotated timestamp

Connection URLs are derived at runtime from these fields and the private network alias. They should not be persisted as duplicate plaintext.

### `database_bindings`

Records automatic injection into consuming application services:

- `id`
- `database_instance_id`
- `environment_id`
- `consumer_service_id`
- `variable_key`
- explicit confirmation when the managed value may replace an existing application variable during deployment
- timestamps

A binding is valid only when both services belong to the same application and environment. Default variable keys are suggested by engine, but the user can rename them.

### `backup_destinations`

Reusable encrypted object-storage configuration:

- `id`
- display name
- provider: `r2` or `s3`
- endpoint
- region
- bucket
- optional prefix
- path-style setting
- optional S3 provider-side encryption mode (`AES256` or `aws:kms`) and KMS key ID
- encrypted access key
- encrypted secret key
- last connection-test status and time
- timestamps

R2 uses the account-specific endpoint and `auto` region. Generic S3 keeps endpoint and region configurable for AWS S3, MinIO, Backblaze B2, Wasabi, and other compatible providers.

### `database_backup_policies`

- `database_instance_id`
- destination ID
- enabled
- schedule
- timezone
- retention count or retention days
- last scheduled run
- next scheduled run

### `database_backups`

- `id`
- unique owning database operation ID
- instance ID
- destination ID when uploaded remotely
- operation status
- trigger kind: manual or scheduled
- object key
- archive format
- checksum
- compressed size
- engine version
- encryption algorithm and encrypted data-key metadata
- error code and safe error message
- started, completed, and expiry timestamps

### `database_operations`

Track long-running provisioning, start, stop, restart, backup, restore, credential rotation, delete, and purge operations. The API should return an operation immediately and let the UI poll or stream status rather than holding an HTTP request open.

## Engine catalog and adapters

Implement a database engine registry in Go. Each adapter owns:

- supported version list and default version;
- digest-pinned image mapping;
- internal port and URL scheme;
- required initialization environment variables;
- persistent data directory;
- readiness command;
- connection URL construction;
- logical backup command;
- restore command;
- graceful stop behavior;
- credential rotation behavior;
- minimum supported memory;
- log redaction rules.

Initial adapters:

| Engine | Private port | Health strategy | Logical backup |
| --- | ---: | --- | --- |
| PostgreSQL | 5432 | `pg_isready` plus authenticated query | `pg_dump` |
| MySQL | 3306 | `mysqladmin ping` plus authenticated query | `mysqldump` |
| MariaDB | 3306 | `mariadb-admin ping` plus authenticated query | `mariadb-dump` |
| MongoDB | 27017 | authenticated `ping` command | `mongodump` |
| Redis | 6379 | authenticated `PING` | RDB snapshot export |
| Valkey | 6379 | authenticated `PING` | RDB snapshot export |

Only tested versions appear in the UI. Patch upgrades can be added to the catalog after backup-and-restore compatibility tests. Major upgrades require an explicit migration workflow and are not performed by changing an image tag in place.

## Private networking

Create one user-defined Docker bridge network per application environment:

`hostforge-env-<environment-id>`

User-defined bridge networks provide container-name and alias DNS resolution. Each database instance receives a stable alias derived from its service identity. Application containers are attached to the environment network in addition to retaining their existing loopback host-port binding for Caddy.

Rules:

- Production application containers attach only to the Production network.
- Staging application containers attach only to the Staging network.
- Database containers publish no host ports.
- Database connection URLs use the stable network alias and engine port.
- Docker network reconciliation runs on server startup and before provisioning.
- Removing an application container does not remove its environment network.
- An environment network is removed only after it has no managed containers and no retained database instances.

Public access is a future phase. It must not be implemented by binding a database directly to `0.0.0.0`.

## Provisioning lifecycle

1. Validate engine, version, environment, resource preset, service name, and bindings.
2. Reserve service, instance, volume, alias, credentials, and operation records transactionally.
3. Ensure the environment network exists.
4. Create the named Docker volume with HostForge ownership labels.
5. Pull the digest-pinned engine image.
6. Start the container with:
   - the persistent data mount;
   - CPU and memory limits;
   - restart policy;
   - engine initialization variables;
   - the private environment network and alias;
   - no published host port;
   - HostForge application, environment, service, and instance labels.
7. Run the engine adapter readiness check.
8. Mark the instance healthy and materialize selected connection bindings.
9. Restart or redeploy consuming services only after explicit user confirmation.
10. Record a platform event and complete the operation.

Provisioning must be idempotent. Retrying after a server restart should adopt correctly labelled containers and volumes instead of creating duplicates.

## Resource presets

Initial defaults:

| Preset | CPU | Memory | Intended use |
| --- | ---: | ---: | --- |
| Development | 0.5 vCPU | 512 MB | Staging, prototypes, and low traffic |
| Standard | 1 vCPU | 1 GB | Small production workloads |
| Performance | 2 vCPU | 4 GB | Heavier workloads |
| Custom | User selected | User selected | Advanced configuration |

Each engine adapter can reject a preset below its safe minimum.

Show storage usage and host free-space warnings, but describe storage as observed usage rather than an enforced quota. Provisioning, restore, and backup operations must stop early when the host would cross the configured minimum-free-disk threshold.

## Credentials and application bindings

- Generate database names and usernames from safe slugs plus random entropy.
- Generate passwords using a cryptographically secure random source.
- Encrypt passwords with HostForge's configured encryption key.
- Percent-encode URL components correctly.
- Never write passwords or full connection URLs to database logs, operation errors, platform events, or container inspection summaries.
- Suggest `DATABASE_URL` for the first relational database and engine-specific names such as `POSTGRES_URL`, `MYSQL_URL`, `MONGODB_URL`, `REDIS_URL`, or `VALKEY_URL` when needed.
- Prevent two active bindings from silently owning the same variable key for one consumer service and environment.
- Resolve managed database bindings during application container startup alongside normal environment variables.
- Managed binding values take precedence only after the UI clearly reports a key conflict and the user confirms replacement.

Credential rotation creates a new generation, changes the credential inside the engine, updates the encrypted record, and then prompts the user to restart affected consumers. If the engine update fails, the stored credential must remain unchanged.

The first release shows private host, port, database, username, and connection status. Password reveal and copyable external connection bundles are reserved for the future public-access feature.

## Backup destinations

### Connect Cloudflare R2

The R2 form asks for:

- account ID;
- bucket;
- access key ID;
- secret access key;
- optional object prefix.

HostForge derives the standard R2 S3 endpoint and uses region `auto`, while allowing a jurisdiction-specific endpoint when required. The connection flow is:

1. Enter credentials.
2. Test bucket access with a scoped write/read/delete probe.
3. Encrypt and save the destination.
4. Offer to enable the default daily backup policy.

The UI should guide users to create a bucket-scoped Object Read & Write credential. A future Cloudflare OAuth integration may reduce manual credential entry, but it is not required for database v1.

### Connect S3-compatible storage

The generic form supports:

- endpoint;
- region;
- bucket;
- access key ID;
- secret access key;
- optional prefix;
- path-style access;
- optional server-side encryption settings.

Use one S3-compatible Go implementation for both R2 and generic S3.

## Backup and restore behavior

- Backups run in short-lived, engine-specific job containers attached to the private environment network.
- Backup jobs receive credentials through memory-only environment injection.
- Stream output into a compressed archive instead of first creating an unbounded plaintext dump.
- Encrypt the compressed stream before it is written locally or uploaded remotely. Use a per-backup data key protected by the HostForge encryption key so rotating object-storage credentials does not affect restore capability.
- Calculate a checksum while streaming.
- Upload to an engine/application/environment/instance/date-based object key.
- Keep safe progress, byte counts, duration, and errors in SQLite.
- Apply retention in HostForge. Provider lifecycle rules can be an additional safeguard but are not the sole source of truth.
- A backup is successful only after the object exists and its size/checksum metadata is recorded.
- Scheduled backup failures create visible events and must not silently disable the next run.

Restore options:

- **Restore into a new service** is the recommended and safest default.
- **Replace current instance** requires a fresh safety backup, stopping consumers, typed confirmation, and a rollback path.
- Restores reject incompatible engine families and unsupported major-version transitions.
- A restored instance receives new runtime credentials unless the user explicitly chooses to preserve compatible credentials.

## Stop, delete, and purge behavior

Stopping a database stops its container but keeps its network identity, credentials, and volume.

Deleting a database:

1. Requires typing the service name.
2. Stops and removes the container.
3. Removes automatic application bindings.
4. Marks the service and instances deleted.
5. Retains volumes and backup metadata for seven days.
6. Shows the purge date and a Restore database action.

Permanent purge:

- runs from a background reconciliation worker after `purge_after`;
- requires no active restore operation;
- removes the Docker volume;
- deletes encrypted credentials;
- preserves a minimal audit event;
- is retryable when Docker is temporarily unavailable.

The existing `DeleteServiceAndRuntime` path must branch on `service_type`. It must never pass a database service through application deployment/image cleanup.

## API outline

### Catalog and creation

- `GET /api/database-engines`
- `POST /api/applications/:applicationID/database-services`
- `GET /api/database-operations/:operationID`

Creation accepts engine, version, target environments, resource preset, optional custom limits, and application connection bindings.

### Instance operations

- `GET /api/database-services/:serviceID`
- `POST /api/database-instances/:instanceID/start`
- `POST /api/database-instances/:instanceID/stop`
- `POST /api/database-instances/:instanceID/restart`
- `POST /api/database-instances/:instanceID/rotate-credentials`
- `GET|POST /api/database-instances/:instanceID/upgrade`
- `DELETE /api/database-services/:serviceID`
- `POST /api/database-services/:serviceID/restore-deleted`
- `DELETE /api/database-services/:serviceID/purge`
- `GET /api/database-instances/:instanceID/logs`
- `GET /api/database-instances/:instanceID/metrics`

### Bindings

- `GET|POST /api/database-instances/:instanceID/bindings`
- `PATCH|DELETE /api/database-bindings/:bindingID`

### Backup configuration and operations

- `GET|POST /api/backup-destinations`
- `PATCH|DELETE /api/backup-destinations/:destinationID`
- `POST /api/backup-destinations/:destinationID/test`
- `GET|PUT /api/database-instances/:instanceID/backup-policy`
- `GET|POST /api/database-instances/:instanceID/backups`
- `POST /api/database-backups/:backupID/restore`
- `DELETE /api/database-backups/:backupID`

All mutations return stable public error codes and record platform events. Secret fields are write-only.

## User interface

### Add Service

Make the Database card functional and open a dedicated wizard:

1. **Engine** — choose engine and supported version.
2. **Environments** — Production, Staging, or both; both are selected by default.
3. **Resources** — Development, Standard, Performance, or Custom.
4. **Connections** — select application services and variable names for each environment.
5. **Backups** — choose an existing destination, connect R2/S3, or continue with a clear warning that remote backups are not configured.
6. **Review** — show the separate instances, volumes, bindings, and estimated host resource allocation.

Provisioning opens an operation screen with durable progress steps rather than a deployment log page.

### Database detail

Keep database navigation persistent across:

- Overview
- Data and connections
- Backups
- Metrics
- Logs
- Settings

The overview should show environment cards independently so a healthy Staging database is not described as degraded because Production is absent or stopped.

### Settings

Include:

- engine and pinned version;
- resource allocation;
- application bindings;
- credential rotation;
- backup policy;
- retention and deletion controls;
- future Public access section marked unavailable with a security explanation.

## Background workers and reconciliation

Add bounded workers for:

- queued database operations;
- scheduled backups;
- health and storage sampling;
- retained-volume purge;
- Docker resource reconciliation after restart.

Use database-backed leases so a future multi-process server cannot run the same operation twice. Server startup must reconcile records against labelled Docker networks, volumes, and containers and surface drift instead of silently recreating potentially destructive resources.

Lease recovery must atomically return the operation and its backup, restore, or upgrade companion record to a retryable state. Backup records are linked to their owning operation so a retry can never select another queued backup for the same instance.

## Delivery phases

Implementation status as of 2026-07-17: Phases 1–5 are implemented in code. The catalog provisions all six digest-pinned engine families with private bindings, leased durable operations, reconciliation, diagnostics, crash-resumable rotation, retained deletion and purge, encrypted streaming R2/S3 backups, schedules, retention, restore-new, and guarded replace-current with a safety-backup rollback. Disk-reserve and transfer-rate admission, observed Docker volume usage, editable provider/binding contracts, and same-version patch-image upgrades with a recent-backup preflight and automatic previous-digest rollback are included in the hardening work. Six-engine runtime lifecycle and restore-drill acceptance on the target VPS remains mandatory before the feature is declared complete.

### Phase 1 — Persistent-resource foundation

- Add `service_type` and database tables.
- Add Docker network, volume, labels, resource-limit, exec, and inspect primitives.
- Add operation records and reconciliation framework.
- Make application containers join their environment network.
- Add deletion guards that protect database volumes.

### Phase 2 — PostgreSQL vertical slice

- Implement the engine registry and PostgreSQL adapter.
- Build the database wizard, provisioning operation UI, detail pages, and private connection binding.
- Support start, stop, restart, logs, health, metrics, rotation, deletion, retention, and purge.
- Validate the architecture end to end before duplicating it across engines.

### Phase 3 — Complete initial engine catalog

- Add MySQL and MariaDB.
- Add MongoDB.
- Add Redis and Valkey.
- Add engine-specific validation, backup, restore, rotation, and version tests.
- Do not declare the database feature complete until every listed engine passes the same lifecycle acceptance suite.

### Phase 4 — Remote backups

- Add the shared S3 client.
- Add Connect Cloudflare R2 and generic S3 settings.
- Add manual backups, schedules, retention, download metadata, restore-new, and replace-current workflows.
- Add low-disk admission checks and failed-backup alerts.

### Phase 5 — Operational hardening

- Add restart recovery and drift reconciliation tests.
- Add upgrade preflight and safe patch-version workflows.
- Add backup restore drills to the VPS acceptance process.
- Add rate limits and concurrency limits for backup/restore jobs.
- Document VPS storage sizing, encryption-key recovery, disaster recovery, and R2/S3 setup.

### Future phase — Secure public database access

Research and design:

- authenticated TCP ingress rather than Caddy HTTP routing;
- per-instance public endpoint allocation;
- database-native TLS or a secure TCP proxy with verified upstream identity;
- firewall allowlists and optional temporary access;
- automated certificate lifecycle;
- connection and authentication audit logs;
- brute-force protection and rate controls;
- copyable host, port, database, username, password, CA certificate, CLI command, and connection URL;
- explicit enable/disable controls and credential revocation.

No database port should be exposed publicly before this phase is completed and reviewed.

## Test and acceptance requirements

### Repository and API tests

- Existing application services migrate to `service_type=application`.
- Application source validation does not run for databases.
- Cross-application and cross-environment bindings are rejected.
- Secret plaintext never appears in JSON, logs, events, or errors.
- Creation and operation retries are idempotent.
- Database deletion cannot call application image cleanup.
- Purge cannot occur before the retention deadline.

### Docker integration tests

For every engine:

- provision and pass readiness;
- persist data across restart and container recreation;
- remain unreachable from the host's public interfaces;
- connect from an application container in the same environment;
- reject access from another environment network;
- rotate credentials and invalidate the old password;
- create a backup and restore verified data;
- survive HostForge server restart and reconciliation;
- delete, restore during retention, and permanently purge.

### Backup destination tests

- R2 endpoint derivation and `auto` region.
- R2 jurisdiction-specific endpoint override.
- Generic S3 path-style and virtual-host-style access.
- Invalid credentials and insufficient bucket permissions.
- Interrupted upload, retry, checksum validation, retention, and restore.
- Destination deletion is blocked while active policies depend on it.

### VPS acceptance

- Upgrade an existing installation without changing application behavior.
- Provision all six engines with digest-pinned images.
- Verify memory and CPU enforcement.
- Verify disk-pressure admission behavior.
- Run a backup to R2 or S3 and restore it into a new service.
- Restart Docker and `hostforge-server`, then verify reconciliation.
- Confirm `ss -lnt` shows no publicly bound database ports.

## Documentation updates required during implementation

- Update `docs/api-v2.md` with database and backup endpoints.
- Replace the planned database shell in `docs/service-types-roadmap.md`.
- Add database storage and backup variables to `docs/operator-guide.md`.
- Add migration, image-pull, backup, restore-drill, and rollback steps to `docs/vps-update.md`.
- Add database lifecycle checks to `docs/v2-staging-acceptance.md`.

## Definition of done

Database services are complete when an operator can provision any initial-catalog engine in Production and Staging, privately connect selected HostForge application services, observe health and resource use, rotate credentials, back up to R2 or S3, restore verified data, and safely delete or recover the service without exposing a database port or risking accidental volume removal.

## Technical references

- [Cloudflare R2 S3-compatible API](https://developers.cloudflare.com/r2/get-started/s3/)
- [Cloudflare R2 authentication and bucket-scoped credentials](https://developers.cloudflare.com/r2/api/tokens/)
- [Cloudflare R2 object lifecycle rules](https://developers.cloudflare.com/r2/buckets/object-lifecycles/)
- [Docker user-defined bridge networking](https://docs.docker.com/engine/network/drivers/bridge/)
