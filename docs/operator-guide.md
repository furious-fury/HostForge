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
