# Operator guide

HostForge targets a Linux VPS. Keep the management server private and expose
applications only through Caddy HTTPS hostnames.

## Installation

```bash
sudo ./scripts/install.sh --with-systemd
```

The installer creates the service user, data directory, systemd unit, and an
environment-file template when one does not already exist. Configure secrets in
`/etc/hostforge/hostforge.env`; never commit that file.

See [`scripts/hostforge-server.env.example`](../scripts/hostforge-server.env.example)
for the configuration reference, including disabled-by-default Railpack/BuildKit
settings.

## Network boundary

- Allow public inbound TCP 80 and 443 for Caddy.
- Keep HostForge on `127.0.0.1:8080` unless an operator deliberately places it
  behind Caddy or an authenticated private network.
- Containers bind only to loopback ports. Never share raw Docker host ports as
  application URLs.
- Point platform or custom DNS hostnames at the VPS before Caddy can obtain
  certificates.

## Operations

- Use `systemctl status hostforge-server` and `journalctl -u hostforge-server`
  for service diagnostics.
- Treat the SQLite database, generated Caddy config, and environment file as
  sensitive backup inputs.
- Follow the [phase 1 validation runbook](./operator-validation-phase1.md) on a
  clean VPS before relying on a deployment path.
- Do not enable `HOSTFORGE_RAILPACK_ENABLED=true` until the Railpack/BuildKit
  smoke test in [ADR 0001](./adr-0001-railpack-buildkit.md) passes.

## Managed database storage and backups

- Database containers use labelled Docker volumes and environment-private bridge networks. They never publish a host port.
- Set `HOSTFORGE_DATABASE_MIN_FREE_DISK_BYTES` to the storage reserve below which provisioning, backup, and restore are rejected. The default is 5 GiB; size it higher than the largest expected restore plus Docker image growth.
- `HOSTFORGE_DATABASE_OPERATION_CONCURRENCY` bounds persistent database workers from 1 to 8 and defaults to 1. Increase it only after measuring disk I/O and memory pressure on the VPS.
- `HOSTFORGE_DATABASE_TRANSFER_MAX_PER_HOUR` sets the global rolling-hour queue limit for backups and restores and defaults to 60.
- Preserve `HOSTFORGE_ENV_ENCRYPTION_KEY`. It protects database credentials, backup-destination credentials, and per-backup data keys. Losing it makes managed credentials and encrypted backups unrecoverable.
- Configure Cloudflare R2 or generic HTTPS S3 storage from Settings, then test the destination before enabling a database backup policy.
- Treat replace-current restore as destructive even though HostForge creates and verifies a safety backup first. Prefer restore-as-copy for drills and investigation.
- Back up `/var/lib/hostforge`, `/etc/hostforge/hostforge.env`, and the object-storage bucket under separate access controls. A SQLite copy without the encryption key is not a complete disaster-recovery set.

## Control-plane durability

Two independent snapshot mechanisms protect `hostforge.db` itself (ADR-0002 §17), separate from the managed-database backups above:

- A pre-migration snapshot runs automatically inside `hostforge-server` whenever a boot has at least one pending schema migration — roughly once per version upgrade. It is a plain file next to `hostforge.db`, named `hostforge.db.pre-migration-<timestamp>.snapshot`, and is never purged automatically; delete old ones by hand once you no longer need them.
- A scheduled snapshot loop takes a `VACUUM INTO` copy on its own interval, tracked in the database and retained for a configurable window. Configure it with `HOSTFORGE_CONTROL_PLANE_SNAPSHOT_INTERVAL_MINUTES` (default 360; `0` disables it), `HOSTFORGE_CONTROL_PLANE_SNAPSHOT_RETENTION_DAYS` (default 14), `HOSTFORGE_CONTROL_PLANE_SNAPSHOT_DIR` (defaults under the data directory), and optionally `HOSTFORGE_CONTROL_PLANE_SNAPSHOT_DESTINATION_ID` to also upload each snapshot to a configured backup destination.
- Restore either kind of snapshot with `scripts/control-plane-restore.sh /path/to/snapshot.sqlite`, run as root with `HF_CONFIRM_CONTROL_PLANE_RESTORE=RESTORE` set. It stops the service, backs up the current database first, installs the snapshot, and restarts.
- A control-plane snapshot contains every secret the database has ever sealed. Back up `HOSTFORGE_ENV_ENCRYPTION_KEY` on a separate path from the snapshots themselves — a snapshot without the matching key is unrecoverable. Restoring one sealed under a different key is not silent: the server fails its encryption canary check at boot and the restore script reports this distinctly (exit code `3`) rather than treating it as a failed restore.
